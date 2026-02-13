package wizard

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ProjectConfig holds all data needed to scaffold a new project.
type ProjectConfig struct {
	ProjectName string
	ServerURL   string
	ParentDir   string
	Agents      []AgentInfo
	CLIPath     string // path to koor-cli binary (empty = skip copy)
}

// AgentConfig holds data needed to scaffold a single agent.
type AgentConfig struct {
	ProjectName  string
	AgentName    string
	Stack        string
	DBType       string // "sqlite", "postgres", "memory" — only for go-api stack
	ServerURL    string
	WorkspaceDir string
	CLIPath      string // path to koor-cli binary (empty = skip copy)
}

// AgentInfo is the per-agent data collected during the wizard.
type AgentInfo struct {
	Name   string
	Stack  string
	DBType string // "sqlite", "postgres", "memory" — only for go-api stack
}

// ScaffoldProject creates the full project directory structure:
// controller dir + all agent dirs.
func ScaffoldProject(cfg ProjectConfig) error {
	slug := Slug(cfg.ProjectName)

	// Build agent summaries for the controller template.
	agents := make([]agentSummary, len(cfg.Agents))
	for i, a := range cfg.Agents {
		agentDir := filepath.Join(cfg.ParentDir, slug+"-"+Slug(a.Name))
		stackTmpl := Registry[a.Stack]
		agents[i] = agentSummary{
			Name:         a.Name,
			Stack:        stackTmpl.DisplayName,
			WorkspaceDir: agentDir,
		}
	}

	// Create controller directory.
	controllerDir := filepath.Join(cfg.ParentDir, slug+"-controller")
	if err := scaffoldController(controllerDir, cfg, agents); err != nil {
		return fmt.Errorf("controller: %w", err)
	}

	// Create each agent directory.
	for _, a := range cfg.Agents {
		agentDir := filepath.Join(cfg.ParentDir, slug+"-"+Slug(a.Name))
		agentCfg := AgentConfig{
			ProjectName:  cfg.ProjectName,
			AgentName:    a.Name,
			Stack:        a.Stack,
			DBType:       a.DBType,
			ServerURL:    cfg.ServerURL,
			WorkspaceDir: agentDir,
			CLIPath:      cfg.CLIPath,
		}
		if err := ScaffoldAgent(agentCfg); err != nil {
			return fmt.Errorf("agent %s: %w", a.Name, err)
		}
	}

	return nil
}

func scaffoldController(dir string, cfg ProjectConfig, agents []agentSummary) error {
	// Create directories for both Claude Code and Cursor IDE.
	dirs := []string{
		filepath.Join(dir, ".claude"),
		filepath.Join(dir, ".cursor"),
		filepath.Join(dir, "plan", "decisions"),
		filepath.Join(dir, "agents"),
		filepath.Join(dir, "status"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	// Write mcp.json for both IDEs.
	mcpData, err := GenerateMCPJSON(cfg.ServerURL)
	if err != nil {
		return fmt.Errorf("generate mcp.json: %w", err)
	}
	for _, mcpPath := range []string{
		filepath.Join(dir, ".claude", "mcp.json"),
		filepath.Join(dir, ".cursor", "mcp.json"),
	} {
		if err := os.WriteFile(mcpPath, mcpData, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", mcpPath, err)
		}
	}

	// Render instructions content (shared between CLAUDE.md and .cursorrules).
	slug := Slug(cfg.ProjectName)
	claudeContent, err := RenderControllerCLAUDEMD(controllerData{
		ProjectName: cfg.ProjectName,
		ProjectSlug: slug,
		ServerURL:   cfg.ServerURL,
		TopicPrefix: slug,
		Agents:      agents,
	})
	if err != nil {
		return fmt.Errorf("render CLAUDE.md: %w", err)
	}

	// Write CLAUDE.md (Claude Code) and .cursorrules (Cursor).
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(claudeContent), 0o644); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".cursorrules"), []byte(claudeContent), 0o644); err != nil {
		return fmt.Errorf("write .cursorrules: %w", err)
	}

	// Write plan/overview.md.
	overviewContent, err := RenderOverviewMD(cfg.ProjectName, agents)
	if err != nil {
		return fmt.Errorf("render overview.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plan", "overview.md"), []byte(overviewContent), 0o644); err != nil {
		return fmt.Errorf("write overview.md: %w", err)
	}

	// Copy koor-cli into controller workspace if available.
	if cfg.CLIPath != "" {
		if _, err := CopyCLI(cfg.CLIPath, dir); err != nil {
			return fmt.Errorf("copy koor-cli: %w", err)
		}
	}

	return nil
}

// ScaffoldAgent creates a single agent workspace.
func ScaffoldAgent(cfg AgentConfig) error {
	// Create directories for both Claude Code and Cursor IDE.
	for _, dir := range []string{
		filepath.Join(cfg.WorkspaceDir, ".claude"),
		filepath.Join(cfg.WorkspaceDir, ".cursor"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	// Write mcp.json for both IDEs.
	mcpData, err := GenerateMCPJSON(cfg.ServerURL)
	if err != nil {
		return fmt.Errorf("generate mcp.json: %w", err)
	}
	for _, mcpPath := range []string{
		filepath.Join(cfg.WorkspaceDir, ".claude", "mcp.json"),
		filepath.Join(cfg.WorkspaceDir, ".cursor", "mcp.json"),
	} {
		if err := os.WriteFile(mcpPath, mcpData, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", mcpPath, err)
		}
	}

	// Look up stack template.
	stackTmpl, ok := Registry[cfg.Stack]
	if !ok {
		stackTmpl = Registry["generic"]
	}

	// Customize instructions for go-api based on DB type.
	instructions := stackTmpl.Instructions
	if cfg.Stack == "go-api" && cfg.DBType != "" {
		instructions = dbInstructions(cfg.DBType)
	}

	slug := Slug(cfg.ProjectName)
	agentSlug := Slug(cfg.AgentName)

	// Render instructions content (shared between CLAUDE.md and .cursorrules).
	claudeContent, err := RenderAgentCLAUDEMD(agentData{
		ProjectName:      cfg.ProjectName,
		ProjectSlug:      slug,
		AgentName:        cfg.AgentName,
		AgentSlug:        agentSlug,
		Stack:            cfg.Stack,
		StackDisplayName: stackTmpl.DisplayName,
		DBType:           cfg.DBType,
		ServerURL:        cfg.ServerURL,
		TopicPrefix:      slug,
		WorkspaceDir:     cfg.WorkspaceDir,
		Instructions:     instructions,
		BuildCmd:         stackTmpl.BuildCmd,
		TestCmd:          stackTmpl.TestCmd,
		DevCmd:           stackTmpl.DevCmd,
	})
	if err != nil {
		return fmt.Errorf("render CLAUDE.md: %w", err)
	}

	// Write CLAUDE.md (Claude Code) and .cursorrules (Cursor).
	if err := os.WriteFile(filepath.Join(cfg.WorkspaceDir, "CLAUDE.md"), []byte(claudeContent), 0o644); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.WorkspaceDir, ".cursorrules"), []byte(claudeContent), 0o644); err != nil {
		return fmt.Errorf("write .cursorrules: %w", err)
	}

	// Write settings.json for Go app stacks.
	if cfg.Stack == "goth" || cfg.Stack == "go-api" {
		settingsData, err := generateSettingsJSON(cfg.Stack, cfg.DBType)
		if err != nil {
			return fmt.Errorf("generate settings.json: %w", err)
		}
		if err := os.WriteFile(filepath.Join(cfg.WorkspaceDir, "settings.json"), settingsData, 0o644); err != nil {
			return fmt.Errorf("write settings.json: %w", err)
		}
	}

	// Copy koor-cli into agent workspace if available.
	if cfg.CLIPath != "" {
		if _, err := CopyCLI(cfg.CLIPath, cfg.WorkspaceDir); err != nil {
			return fmt.Errorf("copy koor-cli: %w", err)
		}
	}

	return nil
}

// generateSettingsJSON returns the content of settings.json for the given stack/DB configuration.
func generateSettingsJSON(stack, dbType string) ([]byte, error) {
	settings := map[string]any{}

	switch stack {
	case "go-api":
		db := map[string]any{}
		switch dbType {
		case "postgres":
			db["type"] = "postgres"
			db["dsn"] = "postgres://localhost:5432/myapp?sslmode=disable"
		case "memory":
			db["type"] = "memory"
		default: // sqlite
			db["type"] = "sqlite"
			db["dsn"] = "./data.db"
		}
		settings["database"] = db
		settings["server"] = map[string]any{
			"bind": "localhost:8080",
		}
	case "goth":
		settings["database"] = map[string]any{
			"type": "sqlite",
			"dsn":  "./data.db",
		}
		settings["server"] = map[string]any{
			"bind": "localhost:3000",
		}
	}

	return json.MarshalIndent(settings, "", "  ")
}

// cliName returns the platform-appropriate koor-cli binary name.
func cliName() string {
	if runtime.GOOS == "windows" {
		return "koor-cli.exe"
	}
	return "koor-cli"
}

// FindCLI searches for the koor-cli binary.
// It checks next to the running executable first, then falls back to PATH.
// Returns the absolute path or empty string if not found.
func FindCLI() string {
	name := cliName()

	// Check next to the running executable.
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Check PATH.
	if p, err := exec.LookPath(name); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}

	return ""
}

// CopyCLI copies the koor-cli binary from srcPath into destDir.
// Returns the destination path.
func CopyCLI(srcPath, destDir string) (string, error) {
	destPath := filepath.Join(destDir, cliName())

	src, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("open source %s: %w", srcPath, err)
	}
	defer src.Close()

	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", fmt.Errorf("create dest %s: %w", destPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copy: %w", err)
	}

	return destPath, nil
}

// dbInstructions returns go-api stack instructions customized for the DB type.
func dbInstructions(dbType string) []string {
	base := []string{
		"Build a REST API using Go stdlib net/http",
		`Use Go 1.22+ routing patterns with wildcards: mux.HandleFunc("GET /api/resource/{id}", handler)`,
		"Return JSON responses with proper Content-Type headers",
		"Write table-driven tests using the testing package",
	}
	switch dbType {
	case "postgres":
		return append(base, "Use PostgreSQL for data persistence (database/sql + github.com/jackc/pgx)")
	case "memory":
		return append(base, "Use in-memory data structures (maps/slices) for data storage — no external database")
	default: // "sqlite"
		return append(base, "Use SQLite via modernc.org/sqlite for data persistence")
	}
}
