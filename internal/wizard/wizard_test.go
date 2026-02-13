package wizard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryHasAllStacks(t *testing.T) {
	expected := []string{"goth", "go-api", "react", "flutter", "c", "generic"}
	for _, id := range expected {
		if _, ok := Registry[id]; !ok {
			t.Errorf("missing stack %q in Registry", id)
		}
	}

	ids := StackIDs()
	if len(ids) != len(expected) {
		t.Errorf("StackIDs() = %d stacks, want %d", len(ids), len(expected))
	}

	// Verify pinned order: goth first, go-api second, rest sorted alphabetically.
	if ids[0] != "goth" {
		t.Errorf("StackIDs()[0] = %q, want %q", ids[0], "goth")
	}
	if ids[1] != "go-api" {
		t.Errorf("StackIDs()[1] = %q, want %q", ids[1], "go-api")
	}
	for i := 3; i < len(ids); i++ {
		if ids[i] < ids[i-1] {
			t.Errorf("StackIDs() not sorted after pinned: %q before %q", ids[i-1], ids[i])
		}
	}
}

func TestControllerCLAUDEMD(t *testing.T) {
	data := controllerData{
		ProjectName: "Test-Project",
		ProjectSlug: "test-project",
		ServerURL:   "http://localhost:9800",
		TopicPrefix: "test-project",
		Agents: []agentSummary{
			{Name: "frontend", Stack: "Go + templ + HTMX", WorkspaceDir: "./test-project-frontend"},
			{Name: "backend", Stack: "Go REST API", WorkspaceDir: "./test-project-backend"},
		},
	}

	content, err := RenderControllerCLAUDEMD(data)
	if err != nil {
		t.Fatal(err)
	}

	checks := []string{
		"Test-Project Controller",
		"http://localhost:9800",
		"Role: Controller",
		"register_instance",
		"test-project.controller.assigned",
		"frontend",
		"backend",
		"NEVER read or modify files in agent workspace directories",
		"setup agents",
		"check requests",
		"./koor-cli activate",
		"./koor-cli events history",
		"./koor-cli state set",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("controller CLAUDE.md missing %q", want)
		}
	}
}

func TestAgentCLAUDEMD(t *testing.T) {
	data := agentData{
		ProjectName:      "Test-Project",
		ProjectSlug:      "test-project",
		AgentName:        "frontend",
		AgentSlug:        "frontend",
		Stack:            "goth",
		StackDisplayName: "Go + templ + HTMX",
		ServerURL:        "http://localhost:9800",
		TopicPrefix:      "test-project",
		WorkspaceDir:     "./test-project-frontend",
		Instructions: []string{
			"Use templ for all HTML templates (.templ files)",
			"Use HTMX for interactivity",
		},
		BuildCmd: "templ generate && go build ./...",
		TestCmd:  "go test ./... -count=1",
		DevCmd:   "go run ./cmd/server",
	}

	content, err := RenderAgentCLAUDEMD(data)
	if err != nil {
		t.Fatal(err)
	}

	checks := []string{
		"Test-Project",
		"frontend Agent",
		"Stack: goth",
		"Go + templ + HTMX",
		"register_instance",
		"test-project.frontend.done",
		"test-project.frontend.request",
		"NEVER read, write, or modify files outside this workspace directory",
		"ALL communication with other agents MUST go through Koor",
		"NEVER ask the user to copy-paste between windows",
		"templ generate && go build ./...",
		"go test ./... -count=1",
		"go run ./cmd/server",
		"Use templ for all HTML templates",
		"Use HTMX for interactivity",
		"./koor-cli activate",
		"./koor-cli state get",
		"./koor-cli events publish",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("agent CLAUDE.md missing %q", want)
		}
	}
}

func TestMCPJSON(t *testing.T) {
	data, err := GenerateMCPJSON("http://localhost:9800")
	if err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		MCPServers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}

	koor, ok := cfg.MCPServers["koor"]
	if !ok {
		t.Fatal("mcp.json missing 'koor' server entry")
	}
	if koor.Type != "http" {
		t.Errorf("mcp.json type = %q, want %q", koor.Type, "http")
	}
	if koor.URL != "http://localhost:9800/mcp" {
		t.Errorf("mcp.json URL = %q, want %q", koor.URL, "http://localhost:9800/mcp")
	}

	// Test trailing slash stripping.
	data2, _ := GenerateMCPJSON("http://localhost:9800/")
	json.Unmarshal(data2, &cfg)
	if cfg.MCPServers["koor"].URL != "http://localhost:9800/mcp" {
		t.Errorf("trailing-slash URL = %q, want %q", cfg.MCPServers["koor"].URL, "http://localhost:9800/mcp")
	}
}

func TestScaffoldProject(t *testing.T) {
	dir := t.TempDir()

	cfg := ProjectConfig{
		ProjectName: "Test-Project",
		ServerURL:   "http://localhost:9800",
		ParentDir:   dir,
		Agents: []AgentInfo{
			{Name: "frontend", Stack: "goth"},
			{Name: "backend", Stack: "go-api"},
		},
	}

	if err := ScaffoldProject(cfg); err != nil {
		t.Fatal(err)
	}

	// Check controller directory structure (both Claude Code and Cursor).
	controllerDir := filepath.Join(dir, "test-project-controller")
	controllerFiles := []string{
		filepath.Join(controllerDir, "CLAUDE.md"),
		filepath.Join(controllerDir, ".cursorrules"),
		filepath.Join(controllerDir, ".claude", "mcp.json"),
		filepath.Join(controllerDir, ".cursor", "mcp.json"),
		filepath.Join(controllerDir, "plan", "overview.md"),
	}
	for _, f := range controllerFiles {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("missing controller file: %s", f)
		}
	}

	controllerDirs := []string{
		filepath.Join(controllerDir, "plan", "decisions"),
		filepath.Join(controllerDir, "agents"),
		filepath.Join(controllerDir, "status"),
	}
	for _, d := range controllerDirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("missing controller dir: %s", d)
		} else if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}

	// Verify CLAUDE.md and .cursorrules have identical content.
	controllerClaude, _ := os.ReadFile(filepath.Join(controllerDir, "CLAUDE.md"))
	controllerCursor, _ := os.ReadFile(filepath.Join(controllerDir, ".cursorrules"))
	if string(controllerClaude) != string(controllerCursor) {
		t.Error("controller CLAUDE.md and .cursorrules should have identical content")
	}

	// Verify both mcp.json files are identical.
	claudeMCP, _ := os.ReadFile(filepath.Join(controllerDir, ".claude", "mcp.json"))
	cursorMCP, _ := os.ReadFile(filepath.Join(controllerDir, ".cursor", "mcp.json"))
	if string(claudeMCP) != string(cursorMCP) {
		t.Error("controller .claude/mcp.json and .cursor/mcp.json should be identical")
	}

	// Check agent directories (both IDEs).
	for _, name := range []string{"frontend", "backend"} {
		agentDir := filepath.Join(dir, "test-project-"+name)
		agentFiles := []string{
			filepath.Join(agentDir, "CLAUDE.md"),
			filepath.Join(agentDir, ".cursorrules"),
			filepath.Join(agentDir, ".claude", "mcp.json"),
			filepath.Join(agentDir, ".cursor", "mcp.json"),
		}
		for _, f := range agentFiles {
			if _, err := os.Stat(f); err != nil {
				t.Errorf("missing agent file %s: %s", name, f)
			}
		}
	}

	// Verify agent CLAUDE.md and .cursorrules are identical.
	agentClaude, _ := os.ReadFile(filepath.Join(dir, "test-project-frontend", "CLAUDE.md"))
	agentCursor, _ := os.ReadFile(filepath.Join(dir, "test-project-frontend", ".cursorrules"))
	if string(agentClaude) != string(agentCursor) {
		t.Error("agent CLAUDE.md and .cursorrules should have identical content")
	}

	// Verify CLAUDE.md content contains sandbox rules.
	if !strings.Contains(string(agentClaude), "NEVER read, write, or modify files outside this workspace directory") {
		t.Error("agent CLAUDE.md missing sandbox rules")
	}

	// Verify settings.json created for Go app stacks (goth, go-api) but not others.
	for _, name := range []string{"frontend", "backend"} {
		settingsPath := filepath.Join(dir, "test-project-"+name, "settings.json")
		if _, err := os.Stat(settingsPath); err != nil {
			t.Errorf("missing settings.json in %s agent dir", name)
		}
	}
}

func TestScaffoldAgent(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "my-project-mobile")

	cfg := AgentConfig{
		ProjectName:  "My-Project",
		AgentName:    "mobile",
		Stack:        "flutter",
		ServerURL:    "http://localhost:9800",
		WorkspaceDir: agentDir,
	}

	if err := ScaffoldAgent(cfg); err != nil {
		t.Fatal(err)
	}

	// Check files exist (both Claude Code and Cursor).
	files := []string{
		filepath.Join(agentDir, "CLAUDE.md"),
		filepath.Join(agentDir, ".cursorrules"),
		filepath.Join(agentDir, ".claude", "mcp.json"),
		filepath.Join(agentDir, ".cursor", "mcp.json"),
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("missing file: %s", f)
		}
	}

	// Check CLAUDE.md has flutter stack content.
	claudeBytes, err := os.ReadFile(filepath.Join(agentDir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(claudeBytes)
	if !strings.Contains(content, "Flutter") {
		t.Error("agent CLAUDE.md missing Flutter stack display name")
	}
	if !strings.Contains(content, "flutter build") {
		t.Error("agent CLAUDE.md missing flutter build command")
	}
	if !strings.Contains(content, "flutter test") {
		t.Error("agent CLAUDE.md missing flutter test command")
	}
}

func TestScaffoldAgentGenericFallback(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "proj-custom")

	cfg := AgentConfig{
		ProjectName:  "Proj",
		AgentName:    "custom",
		Stack:        "nonexistent-stack",
		ServerURL:    "http://localhost:9800",
		WorkspaceDir: agentDir,
	}

	if err := ScaffoldAgent(cfg); err != nil {
		t.Fatal(err)
	}

	claudeBytes, err := os.ReadFile(filepath.Join(agentDir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claudeBytes), "Generic") {
		t.Error("unknown stack should fall back to generic template")
	}
}

func TestScaffoldAgentGoAPIPostgres(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "proj-backend")

	cfg := AgentConfig{
		ProjectName:  "Proj",
		AgentName:    "backend",
		Stack:        "go-api",
		DBType:       "postgres",
		ServerURL:    "http://localhost:9800",
		WorkspaceDir: agentDir,
	}

	if err := ScaffoldAgent(cfg); err != nil {
		t.Fatal(err)
	}

	// Check CLAUDE.md has postgres instructions.
	claudeBytes, _ := os.ReadFile(filepath.Join(agentDir, "CLAUDE.md"))
	content := string(claudeBytes)
	if !strings.Contains(content, "PostgreSQL") {
		t.Error("go-api agent with postgres DBType should mention PostgreSQL in instructions")
	}
	if strings.Contains(content, "modernc.org/sqlite") {
		t.Error("go-api agent with postgres DBType should not mention SQLite")
	}

	// Check settings.json exists and has postgres config.
	settingsBytes, err := os.ReadFile(filepath.Join(agentDir, "settings.json"))
	if err != nil {
		t.Fatal("missing settings.json")
	}
	if !strings.Contains(string(settingsBytes), `"postgres"`) {
		t.Error("settings.json should contain postgres type")
	}
}

func TestScaffoldAgentGoAPISQLite(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "proj-api")

	cfg := AgentConfig{
		ProjectName:  "Proj",
		AgentName:    "api",
		Stack:        "go-api",
		DBType:       "sqlite",
		ServerURL:    "http://localhost:9800",
		WorkspaceDir: agentDir,
	}

	if err := ScaffoldAgent(cfg); err != nil {
		t.Fatal(err)
	}

	claudeBytes, _ := os.ReadFile(filepath.Join(agentDir, "CLAUDE.md"))
	if !strings.Contains(string(claudeBytes), "modernc.org/sqlite") {
		t.Error("go-api agent with sqlite DBType should mention modernc.org/sqlite")
	}

	settingsBytes, err := os.ReadFile(filepath.Join(agentDir, "settings.json"))
	if err != nil {
		t.Fatal("missing settings.json")
	}
	if !strings.Contains(string(settingsBytes), `"sqlite"`) {
		t.Error("settings.json should contain sqlite type")
	}
}

func TestScaffoldAgentGoAPIMemory(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "proj-api")

	cfg := AgentConfig{
		ProjectName:  "Proj",
		AgentName:    "api",
		Stack:        "go-api",
		DBType:       "memory",
		ServerURL:    "http://localhost:9800",
		WorkspaceDir: agentDir,
	}

	if err := ScaffoldAgent(cfg); err != nil {
		t.Fatal(err)
	}

	claudeBytes, _ := os.ReadFile(filepath.Join(agentDir, "CLAUDE.md"))
	content := string(claudeBytes)
	if !strings.Contains(content, "in-memory") {
		t.Error("go-api agent with memory DBType should mention in-memory")
	}
	if strings.Contains(content, "modernc.org/sqlite") {
		t.Error("go-api agent with memory DBType should not mention SQLite")
	}

	settingsBytes, err := os.ReadFile(filepath.Join(agentDir, "settings.json"))
	if err != nil {
		t.Fatal("missing settings.json")
	}
	if !strings.Contains(string(settingsBytes), `"memory"`) {
		t.Error("settings.json should contain memory type")
	}
}

func TestGothAgentGetsSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "proj-frontend")

	cfg := AgentConfig{
		ProjectName:  "Proj",
		AgentName:    "frontend",
		Stack:        "goth",
		ServerURL:    "http://localhost:9800",
		WorkspaceDir: agentDir,
	}

	if err := ScaffoldAgent(cfg); err != nil {
		t.Fatal(err)
	}

	settingsBytes, err := os.ReadFile(filepath.Join(agentDir, "settings.json"))
	if err != nil {
		t.Fatal("missing settings.json for goth agent")
	}
	if !strings.Contains(string(settingsBytes), `"sqlite"`) {
		t.Error("goth settings.json should default to sqlite")
	}
	if !strings.Contains(string(settingsBytes), `"localhost:3000"`) {
		t.Error("goth settings.json should bind to localhost:3000")
	}
}

func TestFlutterAgentNoSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "proj-mobile")

	cfg := AgentConfig{
		ProjectName:  "Proj",
		AgentName:    "mobile",
		Stack:        "flutter",
		ServerURL:    "http://localhost:9800",
		WorkspaceDir: agentDir,
	}

	if err := ScaffoldAgent(cfg); err != nil {
		t.Fatal(err)
	}

	settingsPath := filepath.Join(agentDir, "settings.json")
	if _, err := os.Stat(settingsPath); err == nil {
		t.Error("flutter agent should NOT have settings.json")
	}
}

func TestValidateProjectName(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
	}{
		{"My-Project", true},
		{"truck-wash", true},
		{"", false},
		{"   ", false},
		{"my project", false},
		{"my/project", false},
		{"my\\project", false},
	}
	for _, tc := range tests {
		err := ValidateProjectName(tc.input)
		if tc.ok && err != nil {
			t.Errorf("ValidateProjectName(%q) = %v, want nil", tc.input, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("ValidateProjectName(%q) = nil, want error", tc.input)
		}
	}
}

func TestValidateAgentName(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
	}{
		{"frontend", true},
		{"data-pipeline", true},
		{"", false},
		{"   ", false},
		{"my agent", false},
		{"front/end", false},
		{"back\\end", false},
	}
	for _, tc := range tests {
		err := ValidateAgentName(tc.input)
		if tc.ok && err != nil {
			t.Errorf("ValidateAgentName(%q) = %v, want nil", tc.input, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("ValidateAgentName(%q) = nil, want error", tc.input)
		}
	}
}

func TestSlug(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Truck-Wash", "truck-wash"},
		{"My Project", "my-project"},
		{"already-lower", "already-lower"},
		{"UPPER", "upper"},
	}
	for _, tc := range tests {
		got := Slug(tc.input)
		if got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCopyCLI(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	// Create a fake koor-cli binary.
	srcPath := filepath.Join(srcDir, cliName())
	fakeContent := []byte("fake-koor-cli-binary-content")
	if err := os.WriteFile(srcPath, fakeContent, 0o755); err != nil {
		t.Fatal(err)
	}

	destPath, err := CopyCLI(srcPath, destDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify destination exists and has correct content.
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(fakeContent) {
		t.Errorf("copied content mismatch: got %q, want %q", got, fakeContent)
	}

	// Verify filename.
	if filepath.Base(destPath) != cliName() {
		t.Errorf("expected %s, got %s", cliName(), filepath.Base(destPath))
	}
}

func TestScaffoldProjectWithCLI(t *testing.T) {
	dir := t.TempDir()

	// Create a fake koor-cli binary.
	fakeCLI := filepath.Join(dir, cliName())
	if err := os.WriteFile(fakeCLI, []byte("fake-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := ProjectConfig{
		ProjectName: "CLI-Test",
		ServerURL:   "http://localhost:9800",
		ParentDir:   dir,
		CLIPath:     fakeCLI,
		Agents: []AgentInfo{
			{Name: "frontend", Stack: "goth"},
			{Name: "backend", Stack: "go-api"},
		},
	}

	if err := ScaffoldProject(cfg); err != nil {
		t.Fatal(err)
	}

	// Verify koor-cli was copied to controller and all agent dirs.
	slug := Slug(cfg.ProjectName)
	checkDirs := []string{
		filepath.Join(dir, slug+"-controller"),
		filepath.Join(dir, slug+"-frontend"),
		filepath.Join(dir, slug+"-backend"),
	}
	for _, d := range checkDirs {
		cliPath := filepath.Join(d, cliName())
		content, err := os.ReadFile(cliPath)
		if err != nil {
			t.Errorf("koor-cli not found in %s: %v", d, err)
			continue
		}
		if string(content) != "fake-binary" {
			t.Errorf("koor-cli content mismatch in %s", d)
		}
	}
}

func TestOverviewMD(t *testing.T) {
	agents := []agentSummary{
		{Name: "frontend", Stack: "Go + templ + HTMX"},
		{Name: "backend", Stack: "Go REST API"},
	}

	content, err := RenderOverviewMD("Test-Project", agents)
	if err != nil {
		t.Fatal(err)
	}

	checks := []string{
		"Test-Project",
		"Master Plan",
		"frontend",
		"backend",
		"Go + templ + HTMX",
		"Go REST API",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("overview.md missing %q", want)
		}
	}
}
