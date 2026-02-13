# Architecture

How Koor is designed and why.

## The Problem: MCP Token Tax

MCP (Model Context Protocol) routes all tool data through the LLM's context window. When an agent reads shared state via MCP, the data flows:

```
Agent asks MCP tool → MCP server reads DB → data returns → LLM sees it all in context
```

Every byte of that data counts against the context window and costs tokens — even if the agent only needs to check a version number. This is the **MCP Token Tax**: the LLM pays to receive data it doesn't need to reason about.

A single MCP tool call to read a 2 KB API contract costs ~500 tokens. A REST GET costs 0 tokens to the LLM.

## The Solution: Control Plane / Data Plane Split

Koor separates coordination into two planes:

```
Control Plane (MCP)          Data Plane (REST + CLI)
─────────────────────        ──────────────────────
register_instance            GET/PUT/DELETE /api/state/*
discover_instances           GET/PUT/DELETE /api/specs/*
set_intent                   POST /api/events/publish
get_endpoints                GET /api/events/history
                             GET /api/events/subscribe (WS)
                             GET/POST/DELETE /api/instances/*
                             GET/PUT/POST /api/validate/*
~750 tokens total            0 tokens (direct HTTP)
```

The MCP tools handle **discovery** — the LLM learns what endpoints exist and who else is connected. All data movement goes through REST or CLI, bypassing the LLM context window entirely.

This is approximately **35x cheaper** per data operation compared to routing everything through MCP.

## Three Shared Layers

### 1. State Store

Key/value store for shared data. Any content type, versioned, ETag-cached.

**Use cases:** API contracts, configuration, build artifacts, shared decisions.

The state store is intentionally simple — a flat key/value space. No hierarchy, no namespaces. Keys are strings, values are blobs. This keeps the interface clean for both LLMs (who read/write JSON via REST) and humans (who use the CLI or dashboard).

### 2. Spec Registry

Per-project specification storage with validation.

**Use cases:** Component schemas, API schemas, coding standards, type definitions.

Specs differ from state in two ways: they are scoped by project (composite key `project/name`), and they can have validation rules attached. A scanner can push specs automatically, and other agents can validate their output against the rules.

### 3. Event Bus

Pub/sub with SQLite-backed history and WebSocket streaming.

**Use cases:** Change notifications, build/test results, agent lifecycle events.

Events are fire-and-forget with history. Subscribers get real-time delivery; agents that weren't connected can poll the history. Topic patterns use glob matching for flexible subscription.

## Technology Choices

### SQLite (modernc.org/sqlite)

- **Pure Go** implementation — no CGO, cross-compiles to all platforms
- **WAL mode** for concurrent reads during writes
- **5-second busy timeout** for write contention
- **Single file** — trivial backup, no external database server
- **Auto-migration** — tables and indexes created on first run

### WebSocket (nhooyr.io/websocket)

- Used for real-time event streaming only
- One goroutine per subscriber
- 64-event buffer per subscriber (drops on overflow)

### MCP (mark3labs/mcp-go)

- StreamableHTTP transport at `/mcp`
- 4 discovery-only tools
- Standard MCP protocol — works with any compliant client

### go:embed

Dashboard HTML/CSS/JS is compiled into the server binary. No external files to deploy.

## Binary Architecture

```
koor-server (single binary)
├── REST API server (port 9800)
│   ├── /api/state/*
│   ├── /api/specs/*
│   ├── /api/events/*
│   ├── /api/instances/*
│   ├── /api/validate/*
│   ├── /api/metrics
│   ├── /mcp (StreamableHTTP)
│   └── /health
├── Dashboard server (port 9847)
│   ├── Embedded static files
│   └── API proxy (/api/* → port 9800)
├── Event pruning goroutine
│   └── Prunes to 1000 events every 60s
└── SQLite database
    └── {data_dir}/data.db (WAL mode)
```

```
koor-cli (single binary)
├── Config management (./settings.json)
├── HTTP client (REST API calls)
└── Polling event subscriber (fallback)
```

## Dependencies

| Dependency | Version | Purpose |
|------------|---------|---------|
| `modernc.org/sqlite` | — | Pure-Go SQLite driver |
| `nhooyr.io/websocket` | — | WebSocket for event streaming |
| `mark3labs/mcp-go` | — | MCP server library |
| `google/uuid` | — | UUID generation for instance IDs |

No CGO. `CGO_ENABLED=0` builds produce fully static binaries.

## Auth Model

Simple Bearer token authentication:

- Server starts with optional `--auth-token`
- If set, all requests (except `/health`) must include `Authorization: Bearer <token>`
- If not set, all requests pass through (local mode)
- The MCP endpoint is behind the same auth middleware
- Instance tokens (from registration) are separate from the server auth token — they are stored per-instance but not currently used for auth

## What Koor Is NOT

- **Not an LLM framework** — it doesn't run or manage LLM sessions
- **Not a message queue** — events are persisted but the bus is simple pub/sub, not a guaranteed delivery system
- **Not a database** — the state store is for coordination data, not application data
- **Not vendor-locked** — any MCP client, any HTTP client, any LLM provider

## The "Redis for AI Coding Agents" Analogy

Redis gives web applications a shared, fast, in-memory data layer. Koor gives AI coding agents a shared coordination layer:

| Redis | Koor |
|-------|------|
| Key/value store | State store |
| Pub/sub channels | Event bus with history |
| — | Spec registry with validation |
| — | Instance registry with discovery |
| — | MCP discovery interface |

The analogy isn't perfect (Koor uses SQLite, not memory), but the role is the same: a lightweight shared layer that multiple independent processes coordinate through.
