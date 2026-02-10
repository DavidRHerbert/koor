# Multi-Agent Workflow

How to coordinate multiple LLM coding agents across IDE instances using Koor.

## The Pattern

Multiple IDE instances — Claude Code, Cursor, or any MCP-capable LLM — coordinated through Koor:

- **Controller** — Orchestrator. Holds the master plan as local files. Assigns tasks, approves requests, tracks progress.
- **Frontend** — Builds the UI. Reads tasks from Koor, publishes completion events, requests changes through Koor.
- **Backend** — Builds the API. Same pattern as Frontend.

The user is the **approver and director**, not the messenger. Agents communicate through Koor; the user says "next", "yes", or "no".

## How It Works

```
                    ┌─────────────┐
                    │  Koor Server │
                    │  State/Events│
                    └──────┬──────┘
                           │ MCP + REST
              ┌────────────┼────────────┐
              │            │            │
        ┌─────┴─────┐ ┌───┴───┐ ┌─────┴─────┐
        │ Controller │ │Frontend│ │  Backend  │
        │  (plan +   │ │(templ +│ │ (Go API + │
        │  decisions)│ │ HTMX)  │ │  SQLite)  │
        └─────┬─────┘ └───┬───┘ └─────┬─────┘
              │            │            │
              └────────────┼────────────┘
                         User
                   (switches windows,
                    says "next"/"yes"/"no")
```

1. **Controller** writes task assignments to Koor state
2. **Agents** read their tasks from Koor when the user says "next"
3. **Agents** publish events to Koor when done or when they need something
4. **Agents** tell the user where to go next ("Go to Controller and say 'check requests'")
5. **User** switches windows and types simple commands — never copy-pastes

## Setup

### 1. Start Koor

```bash
koor-server --bind :9800 --dashboard :9847
```

### 2. Run the wizard

`koor-wizard` scaffolds the entire project interactively — Controller + all agent workspaces:

```bash
koor-wizard
```

The wizard will ask for:
- Project name (e.g. `Truck-Wash`)
- Koor server URL
- Number of agents
- Each agent's name and stack (Go + templ + HTMX, Go REST API, React, Flutter, C, Generic)

It generates everything:

```
truck-wash-controller/
├── .claude/mcp.json             # Claude Code MCP connection
├── .cursor/mcp.json             # Cursor MCP connection
├── CLAUDE.md                    # Controller role instructions (Claude Code)
├── .cursorrules                 # Controller role instructions (Cursor)
├── plan/
│   ├── overview.md              # Master plan (edit this!)
│   └── decisions/               # Decision log (grows)
├── agents/
└── status/

truck-wash-frontend/
├── .claude/mcp.json
├── .cursor/mcp.json
├── CLAUDE.md
└── .cursorrules                 # Includes sandbox rules + stack instructions

truck-wash-backend/
├── .claude/mcp.json
├── .cursor/mcp.json
├── CLAUDE.md
└── .cursorrules
```

Each agent's instruction files include strict **sandbox rules** that keep the agent in its own workspace directory, enforce all cross-agent communication through Koor, and prevent copy-paste relay between windows.

The plan is **plain files** — editable, visible, version-controlled. Not stored in Koor.

### 3. IDE support

The wizard generates config for both Claude Code and Cursor:

| IDE | Rules file | MCP config |
|-----|-----------|------------|
| Claude Code | `CLAUDE.md` | `.claude/mcp.json` |
| Cursor | `.cursorrules` | `.cursor/mcp.json` |

You can mix IDEs across workspaces — e.g. Controller in Claude Code, frontend agent in Cursor (great for DOM work), backend in Claude Code. Multiple IDEs can even open the same workspace simultaneously.

MCP tools available in all IDEs: `register_instance`, `discover_instances`, `set_intent`, `get_endpoints`, `propose_rule`.

For data operations (reading tasks, publishing events), agents use `koor-cli` via Bash — this is by design, keeping the LLM context window clean.

### 4. Open workspaces and start

For each workspace:

1. Open the directory in your IDE
2. Say "next"

The agent registers with Koor, reads its task, and starts working. Start with the Controller to assign initial tasks ("setup agents"), then open each agent workspace.

## The User's Vocabulary

| Word | Where | What happens |
|------|-------|-------------|
| **"setup agents"** | Controller | Generate CLAUDE.md + MCP config for each agent |
| **"next"** | Any agent | Agent checks Koor for its task/events and proceeds |
| **"yes"** | Controller | Approve a pending request |
| **"no"** | Controller | Reject a pending request |
| **"check requests"** | Controller | Review pending requests from other agents |
| **"status"** | Controller | Get overview of all agents' progress |

## Example Flow

### Backend completes a feature

```
[Backend window]
Backend: "Done with trucks CRUD. Go to Frontend and say 'next'."
  (Backend has published a done event and updated Koor state)

[Frontend window]
User: "next"

Frontend:
- Checks Koor events, sees Backend completed trucks CRUD
- Reads API contract from Koor
- Continues wiring up the truck list
```

### Frontend needs a new endpoint

```
[Frontend window]
Frontend: "I need a PATCH endpoint. Published request to Koor.
           Go to Controller and say 'check requests'."

[Controller window]
User: "check requests"

Controller:
- Reads the request from Koor events
- Evaluates against the plan
- "Frontend wants PATCH /api/wash-cycles/{id}/status. Approve? yes/no"

User: "yes"

Controller:
- Updates plan/api-contract.md
- Logs decision in plan/decisions/
- Writes new task to Koor state for Backend
- "Approved. Go to Backend and say 'next'."

[Backend window]
User: "next"

Backend:
- Reads task from Koor
- Implements the endpoint
- "Done. Go to Frontend and say 'next'."
```

## What Koor Provides

| Primitive | Role in multi-agent workflow |
|-----------|---------------------------|
| **MCP** | Agents register and discover each other on startup |
| **State** | Task assignments — agents read when user says "next" |
| **Events** | Done/request/approval notifications between agents |
| **Validation rules** | Automated code quality checks across all agents |
| **Event history** | Survives context resets — agents can re-read what happened |
| **Dashboard** | Visual overview at :9847 |

## What the Controller's Files Provide

| File | Purpose |
|------|---------|
| `plan/overview.md` | Master plan (single source of truth) |
| `plan/api-contract.md` | Shared API contract (Controller updates on approvals) |
| `plan/decisions/*.md` | Decision log (grows as project evolves) |
| `status/*.md` | Progress tracking per agent |
| `agents/*.md` | Generated configs for other agents |

## Key Design Decisions

**Plan lives as files, not in Koor.** The Controller's filesystem is the coordination hub. Koor stores coordination state (tasks, events), not documents.

**User approves, never relays.** Agents publish to Koor; other agents read from Koor. The user says "yes/no" at decision points.

**Controller generates everything.** Only the Controller needs manual setup. It generates CLAUDE.md files for all other agents.

**MCP for discovery, REST/CLI for data.** MCP tools register and discover. `koor-cli` reads state and publishes events — keeping the LLM context window clean.

## CLAUDE.md Reference

### Controller

```markdown
## Koor
- Server: http://localhost:9800
- Project: Truck-Wash
- Role: Controller

## On Startup
1. Register with Koor via MCP: name=truck-wash-controller, stack=fullstack
2. Read plan/overview.md — this is the master plan
3. Read plan/api-contract.md — this is the API contract
4. Check events: koor-cli events history --last 10 --topic "truck-wash.*"

## Your Job
You are the orchestrator. The plan files in this directory are the
single source of truth.

When assigning tasks:
- Write to Koor state: koor-cli state put Truck-Wash/backend-task --data '...'
- NEVER ask the user to paste anything to another agent
- The other agent will read its task from Koor when user says "next"
```

### Frontend / Backend

```markdown
## Koor
- Server: http://localhost:9800
- Project: Truck-Wash
- Role: Frontend

## On Startup
1. Register with Koor via MCP: name=truck-wash-frontend, stack=goth
2. Check your task: koor-cli state get Truck-Wash/frontend-task
3. Check recent events: koor-cli events history --last 5 --topic "truck-wash.controller.*"

## Your Job
When the user says "next":
1. Check Koor for your current task
2. Check for Controller approvals in events
3. Proceed with the task

When you finish a feature:
1. Publish: koor-cli events publish truck-wash.frontend.done --data '{"feature":"..."}'
2. Tell the user: "Done. Go to [next agent] and say 'next'."

When you need something from another agent:
1. Publish request to Koor events
2. Tell the user: "Go to Controller and say 'check requests'."
3. DO NOT ask the user to paste your request.
```

## Naming Conventions

### State keys

```
{Project}/frontend-task     → Current Frontend assignment
{Project}/backend-task      → Current Backend assignment
```

### Event topics

```
{project}.controller.*      → Controller decisions and assignments
{project}.frontend.done     → Frontend completed something
{project}.frontend.request  → Frontend requesting a change
{project}.backend.done      → Backend completed something
{project}.backend.request   → Backend requesting a change
```
