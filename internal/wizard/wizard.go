package wizard

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
)

// Options configures the wizard behavior.
type Options struct {
	Accessible bool
}

// Run runs the unified wizard flow.
func Run(opts Options) error {
	var mode string

	modeForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Koor Wizard").
				Description("Scaffold a multi-agent Koor project").
				Options(
					huh.NewOption("Create a new project", "new"),
					huh.NewOption("Add an agent to an existing project", "add"),
				).
				Value(&mode),
		),
	).WithAccessible(opts.Accessible)

	if err := modeForm.Run(); err != nil {
		return err
	}

	switch mode {
	case "new":
		return runNewProject(opts)
	case "add":
		return runAddAgent(opts)
	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}
}

func runNewProject(opts Options) error {
	var (
		projectName string
		serverURL   = "http://localhost:9800"
		parentDir   = "."
		agentCount  int
	)

	// Phase 1: project basics + agent count.
	phase1 := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Description("e.g., Truck-Wash").
				Placeholder("my-project").
				Value(&projectName).
				Validate(ValidateProjectName),
			huh.NewInput().
				Title("Koor server URL").
				Placeholder("http://localhost:9800").
				Value(&serverURL),
			huh.NewInput().
				Title("Parent directory for workspaces").
				Description("All directories will be created here").
				Placeholder(".").
				Value(&parentDir),
		),
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("How many agents?").
				Options(
					huh.NewOption("1", 1),
					huh.NewOption("2", 2),
					huh.NewOption("3", 3),
					huh.NewOption("4", 4),
					huh.NewOption("5", 5),
				).
				Value(&agentCount),
		),
	).WithAccessible(opts.Accessible)

	if err := phase1.Run(); err != nil {
		return err
	}

	// Phase 2: dynamic agent groups.
	agents := make([]AgentInfo, agentCount)
	groups := make([]*huh.Group, 0, agentCount+1)
	for i := range agents {
		idx := i
		groups = append(groups, huh.NewGroup(
			huh.NewInput().
				Title(fmt.Sprintf("Agent %d name", idx+1)).
				Description("e.g., frontend, backend, mobile").
				Placeholder("agent-name").
				Value(&agents[idx].Name).
				Validate(ValidateAgentName),
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Agent %d stack", idx+1)).
				Options(stackOptions()...).
				Value(&agents[idx].Stack),
		))
	}

	// Summary + confirm group.
	var confirmed bool
	groups = append(groups, huh.NewGroup(
		huh.NewConfirm().
			Title(buildNewProjectSummary(projectName, serverURL, parentDir, agents)).
			Affirmative("Yes, create it").
			Negative("Cancel").
			Value(&confirmed),
	))

	phase2 := huh.NewForm(groups...).WithAccessible(opts.Accessible)
	if err := phase2.Run(); err != nil {
		return err
	}

	if !confirmed {
		fmt.Println("Cancelled.")
		return nil
	}

	// Execute scaffold.
	cfg := ProjectConfig{
		ProjectName: projectName,
		ServerURL:   serverURL,
		ParentDir:   parentDir,
		Agents:      agents,
	}
	if err := ScaffoldProject(cfg); err != nil {
		return fmt.Errorf("scaffold failed: %w", err)
	}

	printNewProjectSuccess(cfg)
	return nil
}

func runAddAgent(opts Options) error {
	var (
		projectName string
		agentName   string
		agentStack  string
		serverURL   = "http://localhost:9800"
		workspaceDir string
	)

	phase1 := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Description("Must match the existing Koor project name").
				Placeholder("my-project").
				Value(&projectName).
				Validate(ValidateProjectName),
			huh.NewInput().
				Title("Agent name").
				Description("e.g., mobile, data-pipeline").
				Placeholder("agent-name").
				Value(&agentName).
				Validate(ValidateAgentName),
			huh.NewSelect[string]().
				Title("Stack").
				Options(stackOptions()...).
				Value(&agentStack),
			huh.NewInput().
				Title("Koor server URL").
				Placeholder("http://localhost:9800").
				Value(&serverURL),
			huh.NewInput().
				Title("Workspace directory").
				Description("Where to create the agent directory").
				Placeholder("./project-agent").
				Value(&workspaceDir),
		),
	).WithAccessible(opts.Accessible)

	if err := phase1.Run(); err != nil {
		return err
	}

	// Default workspace dir if empty.
	if workspaceDir == "" {
		workspaceDir = "./" + Slug(projectName) + "-" + Slug(agentName)
	}

	var confirmed bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(buildAddAgentSummary(projectName, agentName, agentStack, serverURL, workspaceDir)).
				Affirmative("Yes, create it").
				Negative("Cancel").
				Value(&confirmed),
		),
	).WithAccessible(opts.Accessible)

	if err := confirmForm.Run(); err != nil {
		return err
	}

	if !confirmed {
		fmt.Println("Cancelled.")
		return nil
	}

	cfg := AgentConfig{
		ProjectName:  projectName,
		AgentName:    agentName,
		Stack:        agentStack,
		ServerURL:    serverURL,
		WorkspaceDir: workspaceDir,
	}
	if err := ScaffoldAgent(cfg); err != nil {
		return fmt.Errorf("scaffold failed: %w", err)
	}

	printAddAgentSuccess(cfg)
	return nil
}

// --- Helpers ---

func stackOptions() []huh.Option[string] {
	ids := StackIDs()
	opts := make([]huh.Option[string], len(ids))
	for i, id := range ids {
		tmpl := Registry[id]
		opts[i] = huh.NewOption(fmt.Sprintf("%s — %s", tmpl.DisplayName, tmpl.Description), id)
	}
	return opts
}

// ValidateProjectName validates a project name.
func ValidateProjectName(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if strings.ContainsAny(s, " /\\") {
		return fmt.Errorf("project name cannot contain spaces or slashes")
	}
	return nil
}

// ValidateAgentName validates an agent name.
func ValidateAgentName(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("agent name cannot be empty")
	}
	if strings.ContainsAny(s, " /\\") {
		return fmt.Errorf("agent name cannot contain spaces or slashes")
	}
	return nil
}

func buildNewProjectSummary(projectName, serverURL, parentDir string, agents []AgentInfo) string {
	slug := Slug(projectName)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Create project %q?\n\n", projectName))
	b.WriteString(fmt.Sprintf("  Server:     %s\n", serverURL))
	b.WriteString(fmt.Sprintf("  Controller: %s\n", filepath.Join(parentDir, slug+"-controller")))
	for _, a := range agents {
		stackTmpl := Registry[a.Stack]
		b.WriteString(fmt.Sprintf("  Agent:      %s (%s) → %s\n", a.Name, stackTmpl.DisplayName, filepath.Join(parentDir, slug+"-"+Slug(a.Name))))
	}
	return b.String()
}

func buildAddAgentSummary(projectName, agentName, stack, serverURL, workspaceDir string) string {
	stackTmpl := Registry[stack]
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Add agent to project %q?\n\n", projectName))
	b.WriteString(fmt.Sprintf("  Agent: %s (%s)\n", agentName, stackTmpl.DisplayName))
	b.WriteString(fmt.Sprintf("  Server: %s\n", serverURL))
	b.WriteString(fmt.Sprintf("  Directory: %s\n", workspaceDir))
	return b.String()
}

func printNewProjectSuccess(cfg ProjectConfig) {
	slug := Slug(cfg.ProjectName)
	fmt.Printf("\nProject %q created successfully!\n\n", cfg.ProjectName)
	fmt.Println("Created directories:")
	fmt.Printf("  %s/    (Controller)\n", filepath.Join(cfg.ParentDir, slug+"-controller"))
	for _, a := range cfg.Agents {
		stackTmpl := Registry[a.Stack]
		fmt.Printf("  %s/    (%s - %s)\n", filepath.Join(cfg.ParentDir, slug+"-"+Slug(a.Name)), a.Name, stackTmpl.DisplayName)
	}
	fmt.Println()
	fmt.Println("IDE support:")
	fmt.Println("  Claude Code  — reads CLAUDE.md + .claude/mcp.json")
	fmt.Println("  Cursor       — reads .cursorrules + .cursor/mcp.json")
	fmt.Println("  Both IDEs can open the same workspace simultaneously.")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Start Koor server:     koor-server")
	fmt.Printf("  2. Edit the master plan:  %s/plan/overview.md\n", filepath.Join(cfg.ParentDir, slug+"-controller"))
	fmt.Printf("  3. Open %s-controller/ in your IDE\n", slug)
	fmt.Println("     - The AI agent will read its rules and connect to Koor via MCP")
	fmt.Println("     - Say \"setup agents\" to assign initial tasks")
	for i, a := range cfg.Agents {
		fmt.Printf("  %d. Open %s/ in your IDE\n", i+4, filepath.Join(cfg.ParentDir, slug+"-"+Slug(a.Name)))
		fmt.Println("     - Say \"next\" to start working")
	}
	fmt.Printf("\nDashboard: http://localhost:9847\n")
}

func printAddAgentSuccess(cfg AgentConfig) {
	fmt.Printf("\nAgent %q added to project %q!\n\n", cfg.AgentName, cfg.ProjectName)
	fmt.Printf("Created: %s/\n\n", cfg.WorkspaceDir)
	fmt.Println("IDE support:")
	fmt.Println("  Claude Code  — reads CLAUDE.md + .claude/mcp.json")
	fmt.Println("  Cursor       — reads .cursorrules + .cursor/mcp.json")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Open %s/ in your IDE\n", cfg.WorkspaceDir)
	fmt.Println("  2. Say \"next\" — the agent will register with Koor and check for tasks")
	fmt.Println("  3. Go to the Controller and say \"status\" to see the new agent")
}
