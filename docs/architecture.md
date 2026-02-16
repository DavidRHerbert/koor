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
register_instance            GET/PUT/POST/DELETE /api/state/*
discover_instances           GET/PUT/DELETE /api/specs/*
set_intent                   POST /api/events/publish
get_endpoints                GET /api/events/history (time-range)
propose_rule                 GET /api/events/subscribe (WS)
                             GET/POST/DELETE /api/instances/*
                             POST /api/liveness/check
                             GET/PUT/POST /api/validate/*
                             POST/GET/DELETE /api/webhooks/*
                             GET/POST /api/compliance/*
                             POST/GET/DELETE /api/templates/*
                             GET /api/audit, /api/audit/summary
                             GET /api/metrics/agents/*
~750 tokens total            0 tokens (direct HTTP)
```

The MCP tools handle **discovery** — the LLM learns what endpoints exist and who else is connected. All data movement goes through REST or CLI, bypassing the LLM context window entirely.

This is approximately **35x cheaper** per data operation compared to routing everything through MCP.

## Shared Layers

### 1. State Store

Key/value store for shared data. Any content type, versioned with full history, ETag-cached, rollback-capable.

**Use cases:** API contracts, configuration, build artifacts, shared decisions.

The state store is intentionally simple — a flat key/value space. Keys are strings, values are blobs. Every update is archived in `state_history`, enabling version retrieval, JSON diffs between versions, and rollback to any previous state.

### 2. Spec Registry

Per-project specification storage with validation rules, contract validation, and compliance checking.

**Use cases:** Component schemas, API schemas, coding standards, type definitions.

Specs differ from state in two ways: they are scoped by project (composite key `project/name`), and they can have validation rules attached. Rules have lifecycle management (propose/accept/reject), source tracking (local/learned/external), and can be packaged as shareable templates.

### 3. Event Bus

Pub/sub with SQLite-backed history, WebSocket streaming, time-range queries, and webhook notifications.

**Use cases:** Change notifications, build/test results, agent lifecycle events, compliance violations.

Events are fire-and-forget with history. Subscribers get real-time delivery; agents that weren't connected can poll the history with time-range and source filters. Webhooks enable external integrations by POSTing matching events to registered URLs with HMAC signatures.

### 4. Instance Registry

Agent registration, discovery, capabilities, liveness monitoring, and compliance tracking.

**Use cases:** Agent coordination, capability-based discovery, stale agent detection.

Agents register on startup, declare capabilities (e.g. `code-review`, `testing`), and send periodic heartbeats. A background liveness monitor marks agents as stale after 5 minutes of silence. Scheduled compliance checks validate active agents against their project contracts.

### 5. Audit & Observability

Immutable audit log and per-agent metrics in hourly buckets.

**Use cases:** Change tracking, accountability, performance monitoring, debugging.

Every mutating API call is logged with actor, action, resource, detail, and outcome. Agent metrics track call counts, violations, and other counters per hour, enabling trend analysis without external monitoring tools.

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
- 5 discovery and proposal tools (register, discover, set_intent, get_endpoints, propose_rule)
- Standard MCP protocol — works with any compliant client

### go:embed

Dashboard HTML/CSS/JS is compiled into the server binary. No external files to deploy.

## Binary Architecture

```
koor-server (single binary)
├── REST API server (port 9800)
│   ├── /api/state/* (versioned, history, rollback, diff)
│   ├── /api/specs/*
│   ├── /api/events/* (time-range + source filtering)
│   ├── /api/instances/* (capabilities, activation, stale)
│   ├── /api/validate/*
│   ├── /api/contracts/*
│   ├── /api/rules/*
│   ├── /api/webhooks/*
│   ├── /api/compliance/*
│   ├── /api/templates/*
│   ├── /api/audit, /api/audit/summary
│   ├── /api/metrics, /api/metrics/agents/*
│   ├── /mcp (StreamableHTTP)
│   └── /health
├── Dashboard server (port 9847)
│   ├── Embedded static files
│   └── API proxy (/api/* → port 9800)
├── Background goroutines
│   ├── Event pruning (every 60s, caps at 1000)
│   ├── Liveness monitor (every 60s, stale after 5m)
│   ├── Webhook dispatcher (event-driven)
│   └── Compliance scheduler (every 5m)
├── Audit log (immutable, append-only)
├── Agent metrics (hourly buckets)
└── SQLite database
    └── {data_dir}/data.db (WAL mode)
```

```
koor-cli (single binary)
├── Config management (./settings.json)
├── HTTP client (REST API calls)
├── State operations (get, set, delete, history, rollback, diff)
├── Webhook, compliance, template, audit management
└── Polling event subscriber (fallback)
```

## Dependencies

| Dependency | Version | Purpose |
|------------|---------|---------|
| `modernc.org/sqlite` | — | Pure-Go SQLite driver |
| `nhooyr.io/websocket` | — | WebSocket for event streaming |
| `mark3labs/mcp-go` | — | MCP server library |
| `google/uuid` | — | UUID generation for instance IDs |
| `charmbracelet/huh` | — | TUI forms for koor-wizard |

No CGO. `CGO_ENABLED=0` builds produce fully static binaries.

## Internal Packages

| Package | Purpose |
|---------|---------|
| `state` | Key/value store with versioned history and rollback |
| `specs` | Per-project specification registry with validation rules |
| `events` | Pub/sub event bus with SQLite history and WebSocket streaming |
| `instances` | Agent instance registration, discovery, and capabilities |
| `liveness` | Background agent health monitoring (stale detection) |
| `webhooks` | Event-driven HTTP notifications with HMAC signing |
| `compliance` | Scheduled contract validation across active agents |
| `templates` | Shareable template bundles for rules and contracts |
| `audit` | Immutable append-only audit log |
| `observability` | Per-agent metric aggregation in hourly buckets |
| `contracts` | API contract storage and JSON Schema validation |
| `mcp` | MCP server with StreamableHTTP transport |
| `wizard` | Project scaffolding logic for koor-wizard TUI |
| `server` | HTTP server, routing, and handler coordination |
| `db` | SQLite database initialization and migrations |

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
| Key/value store | State store (versioned, with history & rollback) |
| Pub/sub channels | Event bus with history, time-range queries & webhooks |
| — | Spec registry with validation & compliance |
| — | Instance registry with discovery, capabilities & liveness |
| — | Shareable templates & immutable audit trail |
| — | MCP discovery interface |

The analogy isn't perfect (Koor uses SQLite, not memory), but the role is the same: a lightweight shared layer that multiple independent processes coordinate through.
