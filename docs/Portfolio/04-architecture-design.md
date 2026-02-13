# Chapter 4: Architecture Design

## Design Principles

Five principles guided every decision:

1. **Single binary, zero external dependencies** — Download, run, done. No Redis, no Postgres, no Docker required.
2. **Any LLM with Bash can use it** — If an agent can run curl, it has a client. No SDK required.
3. **Token-aware by design** — MCP for discovery only. Data moves through REST.
4. **Language-agnostic** — Koor knows nothing about Go, templ, React, or Python. It stores and serves opaque JSON blobs.
5. **Deploy in 30 seconds** — The "time to first value" must be under a minute.

## The Three Shared Layers

Koor provides three coordination primitives. Each solves a distinct problem:

### 1. State Store

A key/value store for shared data. Keys are strings, values are opaque blobs (any content type, defaults to JSON).

**What it stores:** API contracts, configuration, build artifacts, shared decisions — anything two or more agents need to agree on.

**Why it's simple:** A flat key/value space with no hierarchy, no namespaces, no query language. Auto-incrementing versions and SHA-256 hashes for ETag caching. The simplicity is deliberate — LLMs work better with predictable, flat interfaces than with complex nested structures.

### 2. Spec Registry

Per-project specification storage with validation rules.

**What it stores:** Component schemas, API schemas, coding standards, type definitions — structured specifications that define "how things should be."

**Why it's separate from state:** Specs are scoped by project (composite key `project/name`) and can have validation rules attached. A scanner can push specs automatically, and other agents can validate their output against the rules. State is general-purpose; specs are structured truth with enforcement.

### 3. Event Bus

Pub/sub with SQLite-backed history and WebSocket streaming.

**What it does:** Agents publish events to dot-separated topics (`api.change.contract`, `build.completed`). Other agents subscribe with glob patterns (`api.*`). Events are persisted to history for agents that weren't connected when an event was published.

**Why history matters:** In a multi-agent system, agents start at different times. An agent that connects 10 minutes after a contract change still needs to know about it. History makes events durable without requiring a full message queue.

## Technology Choices

### Go

Single binary compilation. Cross-compiles to every platform with `CGO_ENABLED=0`. Standard library HTTP server is production-quality. No runtime dependencies.

The alternative was Rust (also single binary) but Go was chosen for familiarity and because the existing W2C MCP server is Go — the ecosystem knowledge transfers.

### SQLite (modernc.org/sqlite)

Pure Go implementation — no CGO, no C compiler needed, cross-compiles cleanly. WAL (Write-Ahead Logging) mode enables concurrent reads during writes. 5-second busy timeout handles write contention.

A single file (`data.db`) is trivial to backup, move, and inspect. No database server to manage. The entire coordination state fits comfortably in SQLite for the target use case (dozens of agents, not thousands).

Rejected alternatives:
- **BoltDB/bbolt:** Key/value only, no SQL for flexible queries
- **PostgreSQL/MySQL:** External dependency, defeats single-binary goal
- **Redis:** External dependency, and Koor IS the Redis-like layer

### nhooyr.io/websocket

Mature WebSocket library for real-time event streaming. One goroutine per subscriber with a 64-event buffer. The only WebSocket use case is event subscription — everything else is plain HTTP.

### mark3labs/mcp-go

The same MCP library used by the existing W2C AI MCP server. StreamableHTTP transport at `/mcp`. Proven in production.

### go:embed

Dashboard HTML/CSS/JS compiled directly into the server binary. No external files to deploy, no static file server to configure. The dashboard is available the moment the server starts.

## API Surface Design

21 REST endpoints across 7 categories, plus the MCP endpoint and dashboard:

| Category | Endpoints | Purpose |
|----------|-----------|---------|
| Health | 1 | `GET /health` (no auth) |
| State | 4 | CRUD on key/value pairs |
| Specs | 4 | CRUD on project/name specifications |
| Events | 3 | Publish, history, WebSocket subscribe |
| Instances | 5 | Register, list, get, heartbeat, deregister |
| Validation | 3 | List rules, set rules, validate content |
| Metrics | 1 | Server statistics |
| MCP | 1 | StreamableHTTP transport (4 tools) |

Every endpoint returns JSON. Every error includes a code and message. The design is intentionally boring — standard REST, standard HTTP methods, standard status codes. LLMs parse standard patterns better than novel ones.

## What NOT to Build

Scope control was as important as feature selection. I explicitly excluded:

- **Agent spawning** — Agent Teams and Agent SDK handle this
- **Workflow engine** — LangGraph, CrewAI, and Agent Teams do orchestration
- **LLM proxy** — MCP Gateways exist for this
- **File sync** — Git already works
- **Chat UI** — This is a developer tool, not a chat app
- **Plugins** — v1 ships what it ships
- **RBAC** — Bearer tokens are sufficient for v1
- **Persistent queues** — Events are pub/sub with history, not guaranteed delivery

Each exclusion was a system I considered building and decided against. The resulting scope is minimal but complete — every included feature serves the coordination use case directly.

## Deployment Tiers

Same binary, different flags:

**Tier 1 — Local:** `./koor-server` (localhost:9800, no auth, ./data.db). The default for solo development. Zero configuration.

**Tier 2 — LAN:** `./koor-server --bind 0.0.0.0:9800 --auth-token secret123`. For team setups where agents run on different machines on the same network.

**Tier 3 — Cloud:** Same binary behind a reverse proxy (nginx, Caddy) for TLS. Or in Docker with a mounted volume for the database.

The same binary serves all three tiers. No separate "enterprise edition," no feature flags, no config complexity.

[Next: Chapter 5 — Ecosystem Design](05-ecosystem-design.md)
