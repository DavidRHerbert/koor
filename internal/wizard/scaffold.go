package wizard

import (
	"fmt"
	"os"
	"path/filepath"
)

// ProjectConfig holds all data needed to scaffold a new project.
type ProjectConfig struct {
	ProjectName string
	ServerURL   string
	ParentDir   string
	Agents      []AgentInfo
}

// AgentConfig holds data needed to scaffold a single agent.
type AgentConfig struct {
	ProjectName  string
	AgentName    string
	Stack        string
	ServerURL    string
	WorkspaceDir string
}

// AgentInfo is the per-agent data collected during the wizard.
type AgentInfo struct {
	Name  string
	Stack string
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
			ServerURL:    cfg.ServerURL,
			WorkspaceDir: agentDir,
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
		ServerURL:        cfg.ServerURL,
		TopicPrefix:      slug,
		WorkspaceDir:     cfg.WorkspaceDir,
		Instructions:     stackTmpl.Instructions,
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

	return nil
}
