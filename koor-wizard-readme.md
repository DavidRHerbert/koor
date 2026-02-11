# koor-wizard — How It Works

## What It Does

`koor-wizard` is an interactive TUI (Terminal UI) tool that scaffolds multi-agent Koor projects. It creates workspace directories for a **Controller** and one or more **Agents**, each pre-configured with:

- `CLAUDE.md` — instructions for Claude Code
- `.cursorrules` — identical content for Cursor IDE
- `.claude/mcp.json` — MCP server config (Claude Code)
- `.cursor/mcp.json` — MCP server config (Cursor)
- `koor-cli` / `koor-cli.exe` — CLI binary for data-plane operations (if found)

The Controller gets an additional `plan/overview.md` master plan template.

## Entry Point

```
cmd/koor-wizard/main.go
```

A minimal `main()` that parses a `--accessible` flag (disables TUI chrome for screen readers) and calls `wizard.Run()`.

```bash
go build -o koor-wizard ./cmd/koor-wizard
./koor-wizard
./koor-wizard --accessible   # plain text mode
```

## Package Structure

All logic lives in `internal/wizard/`:

| File | Purpose |
|------|---------|
| `wizard.go` | TUI flow — interactive prompts, validation, orchestration |
| `scaffold.go` | File system operations — create dirs, write files, copy koor-cli |
| `claude_md.go` | Go `text/template` templates for CLAUDE.md (controller + agent + overview) |
| `templates.go` | Stack registry — predefined stack configs (goth, go-api, react, etc.) |
| `mcp_json.go` | Generates `.claude/mcp.json` / `.cursor/mcp.json` content |
| `wizard_test.go` | Tests for templates, scaffolding, validation, CLI copy |

## Two Modes

The wizard offers two modes at startup:

### 1. Create a New Project

Prompts for:
- Project name (e.g., `Truck-Wash`)
- Koor server URL (default: `http://localhost:9800`)
- Parent directory (default: `.`)
- Number of agents (1-5)
- For each agent: name + stack selection

Creates this directory structure:
```
parent-dir/
  truck-wash-controller/     # Controller workspace
    CLAUDE.md
    .cursorrules
    .claude/mcp.json
    .cursor/mcp.json
    plan/overview.md
    plan/decisions/
    agents/
    status/
    koor-cli.exe             # if found

  truck-wash-frontend/       # Agent workspace
    CLAUDE.md
    .cursorrules
    .claude/mcp.json
    .cursor/mcp.json
    koor-cli.exe             # if found

  truck-wash-backend/        # Agent workspace
    (same structure)
```

### 2. Add an Agent to an Existing Project

Prompts for:
- Existing project name
- New agent name + stack
- Server URL
- Workspace directory

Creates a single agent workspace directory.

## Stack Registry

Defined in `templates.go`. Each stack provides:

| Field | Example (goth) |
|-------|----------------|
| `DisplayName` | `Go + templ + HTMX` |
| `Description` | `Full-stack Go with templ templates and HTMX interactivity` |
| `BuildCmd` | `templ generate && go build ./...` |
| `TestCmd` | `go test ./... -count=1` |
| `DevCmd` | `go run ./cmd/server` |
| `Instructions` | Stack-specific rules injected into CLAUDE.md |

Available stacks:
- **goth** — Go + templ + HTMX (full-stack Go)
- **go-api** — Go REST API with SQLite
- **react** — React with Vite
- **flutter** — Flutter cross-platform
- **c** — C with make/cmake
- **generic** — Language-agnostic fallback

Unknown stacks automatically fall back to `generic`.

## CLAUDE.md Templates

Defined in `claude_md.go` using Go's `text/template`.

### Controller Template

The Controller CLAUDE.md tells the AI agent to:
1. Register with Koor via MCP (`register_instance`)
2. Activate via CLI (`./koor-cli activate <instance-id>`)
3. Read `plan/overview.md` — the master plan
4. Check events for pending requests
5. Orchestrate agents by writing tasks to Koor state and publishing events

Key commands: `setup agents`, `check requests`, `status`, `next`, `yes`, `no`

### Agent Template

Each Agent CLAUDE.md tells the AI agent to:
1. Register with Koor via MCP (`register_instance`)
2. Activate via CLI (`./koor-cli activate <instance-id>`)
3. Check its task from Koor state
4. Do the work
5. Publish `done` events when finished
6. Publish `request` events when it needs something from another agent

### Sandbox Rules (Critical)

Both templates include strict sandbox rules:
- **Controller:** NEVER read/modify agent workspace files
- **Agents:** NEVER read/write outside their workspace directory
- **All:** ALL communication goes through Koor (state + events), NEVER ask user to copy-paste

## koor-cli Distribution

Added in Phase 8 to solve the problem of agents failing silently when `koor-cli` is missing.

### FindCLI()

Searches for the `koor-cli` binary in two places:
1. Next to the running `koor-wizard` executable
2. In the system PATH

Returns the absolute path, or empty string if not found.

### CopyCLI()

Copies the binary into each workspace directory with `0o755` permissions.
Platform-aware: appends `.exe` on Windows.

### Behavior

- If found: copied into controller + all agent directories
- If not found: warning printed, scaffolding continues, agents will fail at activate step and report the problem

## Agent Activation Flow

After scaffolding, each AI agent follows this startup sequence:

```
1. Register via MCP: register_instance -> gets instance_id
   Dashboard shows: agent-name [PENDING] (amber badge)

2. Activate via CLI: ./koor-cli activate <instance-id>
   Dashboard shows: agent-name [ACTIVE] (green badge)
   If ./koor-cli fails -> agent tells user: "koor-cli not available"

3. Check task: ./koor-cli state get Project/agent-task
4. Proceed with work...
```

## MCP JSON

Generated by `mcp_json.go`. Points both Claude Code and Cursor to the Koor MCP endpoint:

```json
{
  "mcpServers": {
    "koor": {
      "url": "http://localhost:9800/mcp"
    }
  }
}
```

Trailing slashes on the server URL are automatically stripped.

## Validation

- **Project names:** Cannot be empty, no spaces or slashes
- **Agent names:** Same rules as project names
- **Slug conversion:** `Slug("Truck-Wash")` -> `"truck-wash"` (lowercase, spaces to hyphens)

## Dependencies

- `github.com/charmbracelet/huh` — TUI form library (select menus, text inputs, confirmations)
- Go stdlib: `text/template`, `encoding/json`, `io`, `os`, `os/exec`, `runtime`

## Building

```bash
# Build the wizard
go build -o koor-wizard ./cmd/koor-wizard

# Build the CLI (needed for distribution)
go build -o koor-cli ./cmd/koor-cli

# Place koor-cli next to koor-wizard so FindCLI() finds it automatically
```

## Tests

Run wizard tests:
```bash
go test ./internal/wizard/... -v -count=1
```

Tests cover:
- Stack registry completeness and sorted order
- Controller CLAUDE.md template rendering
- Agent CLAUDE.md template rendering
- MCP JSON generation (including trailing slash handling)
- Full project scaffolding (directory structure, file content, IDE parity)
- Single agent scaffolding
- Generic stack fallback for unknown stacks
- Project/agent name validation
- Slug conversion
- koor-cli copy functionality
- Scaffolding with CLI distribution
- Overview.md rendering
