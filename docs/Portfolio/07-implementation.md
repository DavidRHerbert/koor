# Chapter 7: Implementation

## Build Strategy

The implementation followed a 5-phase plan, each phase delivering a usable milestone. The plan was designed for LLM-assisted development — concise specifications with exact API surfaces, database schemas, and test expectations rather than prose descriptions.

The entire build was done with Claude Opus 4.6 as a pair programmer, working from the consolidated plan documents produced in the research phase.

## Phase 1: Core (State + Specs + CLI)

**Milestone:** Share JSON between agents via CLI.

Built:
- SQLite database layer with auto-migration (5 tables, 3 indexes)
- State store (key/value CRUD with versioning and SHA-256 hashing)
- Spec registry (project/name CRUD with versioning)
- REST API server with routing for state and specs endpoints
- Auth middleware (Bearer token, optional)
- `koor-cli` with config management, state commands, and specs commands
- Health endpoint

**Database schema (5 tables):**

| Table | Primary Key | Purpose |
|-------|-------------|---------|
| `state` | `key` | Versioned key/value store |
| `specs` | `(project, name)` | Per-project specifications |
| `events` | `id` (autoincrement) | Event history |
| `instances` | `id` (UUID) | Agent registry |
| `validation_rules` | `(project, rule_id)` | Content validation rules |

Key design decision: WAL mode enabled from the start for concurrent reads during writes. Busy timeout set to 5 seconds for write contention.

## Phase 2: Events + WebSocket

**Milestone:** Pub/sub working between agents.

Built:
- Event bus with SQLite-backed history
- Publish endpoint (POST with topic + data)
- History endpoint (GET with `?last=N&topic=pattern` filtering)
- WebSocket subscription endpoint using nhooyr.io/websocket
- Topic pattern matching using `path.Match` glob syntax
- Background event pruning goroutine (1000 events max, every 60 seconds)
- CLI events commands (publish, history, subscribe with polling fallback)

The CLI subscribe command intentionally uses polling rather than a WebSocket library — keeping `koor-cli` dependency-free. It polls history every 2 seconds and deduplicates by event ID.

## Phase 3: Instances + MCP

**Milestone:** LLM connects via MCP and discovers other agents.

Built:
- Instance registry (register, list, get, heartbeat, deregister)
- UUID-based instance IDs and tokens (via google/uuid)
- MCP transport using mark3labs/mcp-go StreamableHTTP
- 4 MCP tools: `register_instance`, `discover_instances`, `set_intent`, `get_endpoints`
- CLI register and instances commands

The MCP layer is deliberately thin — 4 tools, ~194 lines of code. Each tool does one thing. `get_endpoints` returns the full REST API surface so the LLM knows exactly where to send curl requests.

## Phase 4: Validation + Dashboard

**Milestone:** Validate code against rules. See metrics in a browser.

Built:
- Validation rules storage (per-project, with severity, match_type, pattern, applies_to)
- Three match types: `regex` (forbidden patterns), `missing` (required patterns), `custom` (built-in checks)
- Filename-based rule filtering using glob patterns
- Validate endpoint (POST content, get violations with line numbers)
- Embedded web dashboard using go:embed
- Dashboard serves on separate port with API proxy (avoids CORS)
- Metrics endpoint

## Phase 5: Polish

**Milestone:** Production-ready with caching, pruning, graceful shutdown, and documentation.

Built:
- ETag caching on state GET and specs GET (SHA-256 hash, `If-None-Match` support)
- `X-Koor-Version` response header
- Graceful shutdown on SIGINT/SIGTERM (5-second deadline)
- Config file support (`settings.json`) with search path and explicit `--config` flag
- Config priority: CLI flags > env vars > config file > defaults
- Structured logging (slog with configurable level)
- Server timeouts (read: 10s, write: 30s, idle: 60s)

## Test Coverage

52 tests across all packages:

| Package | Tests | What's Covered |
|---------|-------|---------------|
| `internal/state` | Store CRUD, versioning, hash generation, deletion |
| `internal/specs` | Registry CRUD, versioning, listing by project |
| `internal/events` | Bus publish, subscribe, history, pattern matching, pruning |
| `internal/instances` | Register, discover, heartbeat, deregister, filtering |
| `internal/specs` (validate) | Regex rules, missing rules, custom rules, filename filtering |
| `internal/server` | HTTP handlers, auth middleware, ETag caching, error responses |
| `internal/mcp` | Tool registration, argument extraction, endpoint discovery |

All tests use in-memory SQLite (`db.OpenMemory()`) for speed and isolation.

## Codebase Statistics

| Metric | Value |
|--------|-------|
| Total Go code | ~3,800 lines |
| Source files | ~15 |
| Dependencies (go.mod) | 3 direct (modernc.org/sqlite, nhooyr.io/websocket, mark3labs/mcp-go) |
| Binary size (Linux amd64) | ~20 MB (includes embedded dashboard) |
| Build time | ~10 seconds |
| Test time | ~2 seconds (in-memory SQLite) |

## Cross-Platform Builds

`CGO_ENABLED=0` produces fully static binaries for:

| Platform | Server | CLI |
|----------|--------|-----|
| Linux amd64 | `koor-server-linux-amd64` | `koor-cli-linux-amd64` |
| Linux arm64 | `koor-server-linux-arm64` | `koor-cli-linux-arm64` |
| macOS amd64 | `koor-server-darwin-amd64` | `koor-cli-darwin-amd64` |
| macOS arm64 | `koor-server-darwin-arm64` | `koor-cli-darwin-arm64` |
| Windows amd64 | `koor-server-windows-amd64.exe` | `koor-cli-windows-amd64.exe` |

No CGO means no C compiler needed, no platform-specific build environment. The Go toolchain handles everything.

[Next: Chapter 8 — Results and Reflection](08-results-and-reflection.md)
