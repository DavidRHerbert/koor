# Koor

Koor is a **coordination server** for AI coding agents — infrastructure, not a framework. Like Redis for caching, Koor provides shared state, specs, and events for any LLM agent that can speak REST or MCP. It's agnostic to IDE, LLM provider, and language.

Koor splits **control** (MCP, thin discovery layer) from **data** (REST + CLI, direct access), solving the MCP Token Tax Problem — where all coordination data gets routed through the LLM context window, burning tokens on data the LLM doesn't need to reason about.

## Why Koor Exists

I wanted a programmatic API for IDE LLM coding agents — an external system that can send prompts to, and receive structured responses from, running IDE agents.

The key to effective AI coding is keeping the context window small, so I run an LLM on each of my codebases at project level: one for frontend, one for backend, plus test projects and reference projects — up to 12 or so open at once. I use Cursor, Kilo Code, Claude Code, Antigravity, and others, checking each out for their strengths and weaknesses.

I found myself copying and pasting between IDEs and getting lost. So I built the first version of this app: **MCP-ChattTeam** — a set of workarounds for missing Claude Code APIs, trying to get multiple LLM instances to coordinate across IDEs via MCP hubs, WebSocket bridges, and file watchers.

Then Anthropic shipped Agent SDK and Agent Teams, solving ~80% of the problem natively. The remaining 20% — cross-IDE, cross-LLM, shared state — needed a different architecture. That became Koor.

The architecture centres on a **central authoritative controller** — a documentation-level LLM that doesn't write code itself but orchestrates the project-level IDE LLM instances as a central authority. Each coding agent connects to Koor via MCP regardless of IDE or LLM provider, and the controller coordinates their work through shared state, specs, and events.

**Koor helps everyone stay on plan, conform to the same rules, and stay in sync with each other.**

- **Stay on plan** — specs define scope so each agent knows what to work on
- **Contained to their part** — agents stay scoped to their own project/codebase
- **Conform to the same rules** — validation rules enforce consistency across all agents
- **Stay in sync** — agents share state (API contracts, configs) so they don't diverge from each other
- **Stay aware** — events let agents know what others have done (e.g. "backend changed the API schema")
- **Find each other** — instance discovery so agents know who else is working

The developer is released from remembering what, where, and when — and instead becomes a critic and a system-wide designer.

Furthermore, the developer is free to become an AI slop generator! Conducting countless little test-and-learn, look-see experiments — that's more apps, more files, more for a human to get lost in forgetting the reasons why? But the central controller knows what fits where, and eventually every aspect will converge into a bucket of worth and a bucket of valuable history.

<img src="docs/images/Koor01-600.png" alt="Koor Architecture" width="600">

## What's Genuinely New

Koor combines coordination + shared specifications + contract validation + cross-LLM + project scaffolding + standalone binary. As of February 2026, no single system covers all seven:

| System | Coordination | Shared State | Shared Specs | Contracts | Cross-LLM | Scaffolding | Standalone |
|--------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| Claude Agent Teams | Yes | No | No | No | No | No | No |
| Claude-Flow | Yes | Yes | No | No | No | No | No |
| LangGraph | Yes | Yes | No | No | No | No | No |
| AutoGen | Yes | Yes | No | No | No | No | No |
| A2A Protocol | Yes | No | No | No | Yes | No | No |
| MCP Gateways | No | No | No | No | Yes | No | Yes |
| W2C AI MCP | No | No | Yes | No | Yes* | No | Yes |
| **Koor** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |



## Two Communication Channels

There are two communication channels to Koor:

| | CLI (Data Plane) | MCP (Control Plane) |
|---|---|---|
| **Transport** | REST over HTTP | MCP protocol (SSE stream) |
| **Used by** | The AI agent running commands | The IDE itself (Claude Code / Cursor) |
| **Purpose** | Read/write state, publish/subscribe events, activate | Register instance, discover peers, propose rules, set intent |
| **Connection** | Stateless — one HTTP call per command | Persistent — IDE holds open connection |
| **Who initiates** | Agent says `./koor-cli ...` in terminal | IDE connects automatically via `.claude/mcp.json` |

The split exists because MCP tools appear in the AI's tool palette (like `register_instance`, `discover_instances`) while CLI commands are run as bash (`./koor-cli state get ...`). MCP handles identity/discovery; CLI handles all the actual data work — avoiding the dreaded token tax.

The key insight: Koor is already network-ready by design. Every agent talks to it over HTTP — whether that HTTP goes to localhost or across the internet is just a URL change.

# These do the exact same thing:
./koor-cli state get Project/task
curl http://localhost:9800/api/state/Project/task -H "Authorization: Bearer token"
So why copy a binary instead?
 Because shorter commands = fewer tokens
./koor-cli state get X is ~8 tokens. The equivalent curl with URL, headers, auth, and content-type is ~40 tokens. Agents call this hundreds of times per session. It adds up.
 Auth is handled once + Self-contained workspace + 

## How It Works

1. Run `koor-wizard` — interactive TUI scaffolds your entire project (Controller + agents)
2. Start `koor-server`
3. Open each workspace in your IDE (Claude Code, Cursor, or any MCP-capable LLM)
4. Say **"next"** — each agent registers with Koor, reads its task, and starts working
5. Agents coordinate through Koor — the user **approves, never relays**

`koor-wizard` generates all the config files for both Claude Code and Cursor automatically — CLAUDE.md, .cursorrules, .claude/mcp.json, .cursor/mcp.json — with strict sandbox rules that keep each agent in its own workspace. You can even open multiple IDEs on the same codebase simultaneously.

### User's Vocabulary

| Word | Where | What it does |
|------|-------|-------------|
| **"setup agents"** | Controller | Generate CLAUDE.md + MCP config for each agent |
| **"next"** | Any agent | Check Koor for your task/events and proceed |
| **"yes"** | Controller | Approve the request |
| **"no"** | Controller | Reject the request |
| **"check requests"** | Controller | Look at pending requests in Koor events |
| **"status"** | Controller | Give me an overview of where everything stands |
 format = koor-cli status
 
Six words to set up and run a multi-agent project.

### What Koor Provides

| Primitive | Role |
|-----------|------|
| **MCP** | Agents register and discover each other on startup |
| **State** | Task assignments (agents read when user says "next") |
| **Events** | Done/request/approval notifications (agents check via CLI) |
| **Contracts** | Shared API contracts with schema validation |
| **Validation** | Automated code quality per stack |
| **Token Tax** | Dashboard tracks MCP vs REST calls and token savings |
| **Dashboard** | Visual overview at :9847 |
| **Wizard** | `koor-wizard` scaffolds entire projects interactively |
| **Event history** | Survives context resets — agent can re-read what happened |

### What the Controller's Files Provide

| File | Role |
|------|------|
| plan/overview.md | Master plan (editable, in plain sight) |
| plan/api-contract.md | API contract (Controller updates on approvals) |
| plan/decisions/*.md | Decision log (grows as project evolves) |
| status/*.md | Progress tracking per agent |

See the full guide: **[Multi-Agent Workflow](docs/multi-agent-workflow.md)**

## Quickstart

```bash
# Build all three binaries
go build ./cmd/koor-server
go build ./cmd/koor-cli
go build ./cmd/koor-wizard

# Start the server
./koor-server
# API: localhost:9800  Dashboard: localhost:9847

# Scaffold a new multi-agent project
./koor-wizard
# Interactive TUI: choose project name, agents, stacks
# Generates all directories, CLAUDE.md, .cursorrules, MCP configs

# Or use the CLI directly
./koor-cli state set api-contract --data '{"version":"1.0"}'
./koor-cli state get api-contract
./koor-cli status
```

### IDE Support

The wizard generates config for multiple IDEs automatically:

| IDE | Rules file | MCP config |
|-----|-----------|------------|
| Claude Code | `CLAUDE.md` | `.claude/mcp.json` |
| Cursor | `.cursorrules` | `.cursor/mcp.json` |

Both IDEs can open the same workspace simultaneously — each connects to Koor independently via MCP.

## Documentation

Full documentation is in the [docs/](docs/) folder:

- **[Getting Started](docs/getting-started.md)** — Install, run, first API call in 5 minutes
- **[Multi-Agent Workflow](docs/multi-agent-workflow.md)** — Coordinate multiple LLM agents across IDE instances
- **[Configuration](docs/configuration.md)** — Flags, env vars, config file, priority rules
- **[API Reference](docs/api-reference.md)** — Complete REST API documentation
- **[CLI Reference](docs/cli-reference.md)** — All koor-cli commands
- **[MCP Guide](docs/mcp-guide.md)** — Connect LLM agents via MCP
- **[Events Guide](docs/events-guide.md)** — Pub/sub, WebSocket streaming, patterns
- **[Specs and Validation](docs/specs-and-validation.md)** — Shared specs and validation rules
- **[Deployment](docs/deployment.md)** — Local, LAN, cloud (Docker, systemd, Windows)
- **[Architecture](docs/architecture.md)** — Design decisions and rationale
- **[Troubleshooting](docs/troubleshooting.md)** — Common issues and fixes

## Architecture

```
LLM ──MCP──> koor-server ──> SQLite (WAL mode)
               ▲
Agent ──REST──/
               ▲
CLI ───REST───/
```

- **4 dependencies:** modernc.org/sqlite, nhooyr.io/websocket, mark3labs/mcp-go, charmbracelet/huh
- **Pure Go:** CGO_ENABLED=0, cross-compiles to all platforms
- **3 binaries:** koor-server (embed dashboard via go:embed), koor-cli, koor-wizard

## Development

```bash
go test ./... -v -count=1    # 120 tests
go build ./...               # Build all
```

## Sponsorship

Koor is free and open source. If it's useful to you or your team, please consider sponsoring the project via [GitHub Sponsors](https://github.com/sponsors/DavidRHerbert).

## License

MIT License — see [LICENSE](LICENSE) for details.
