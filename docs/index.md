# Koor Documentation

Lightweight coordination server for AI coding agents — "Redis for AI coding agents."

Koor splits **control** (MCP, thin discovery layer) from **data** (REST + CLI, direct access), solving the MCP Token Tax Problem where coordination data gets routed through the LLM context window, burning tokens on data the LLM doesn't need to reason about.

## Quick Links

| Doc | Description |
|-----|-------------|
| [Getting Started](getting-started.md) | Install, start the server, make your first API call in under 5 minutes |
| [Configuration](configuration.md) | All server flags, environment variables, config file format, and priority rules |
| [API Reference](api-reference.md) | Complete REST API: every endpoint with method, path, request/response body, status codes, and ETag behaviour |
| [CLI Reference](cli-reference.md) | Every `koor-cli` command with flags, examples, and expected output |
| [MCP Guide](mcp-guide.md) | Connect LLM agents via MCP. IDE config snippets for Claude Code, Cursor, and Kilo Code |
| [Events Guide](events-guide.md) | Pub/sub concepts, topic patterns, WebSocket subscriptions, event history, and use cases |
| [Specs and Validation](specs-and-validation.md) | Shared specifications, validation rules (regex, missing, custom), filename filtering, worked examples |
| [Deployment](deployment.md) | Local, LAN, and cloud deployment. Docker, systemd, Windows service, reverse proxy, and backup |
| [Architecture](architecture.md) | Control/data plane split, technology choices, the "Redis for AI coding agents" concept, dependency rationale |
| [Troubleshooting](troubleshooting.md) | Common issues: auth errors, port conflicts, WebSocket problems, stale instances, build issues |

## Core Concepts

**Shared layers:**

1. **State Store** — Key/value store with versioned history, rollback, and JSON diff
2. **Spec Registry** — Per-project specifications with validation rules and compliance checking
3. **Event Bus** — Pub/sub with SQLite history, WebSocket streaming, time-range queries, and webhook notifications
4. **Instance Registry** — Agent registration, capabilities, liveness monitoring, and stale detection
5. **Audit & Observability** — Immutable change log and per-agent metrics in hourly buckets

**Additional features:**

- **Webhooks** — HTTP POST notifications for matching events with HMAC signatures
- **Compliance** — Scheduled contract validation across active agents
- **Templates** — Shareable rule/contract bundles for cross-project reuse
- **Contracts** — API contract storage with JSON Schema validation

**Two access methods:**

- **MCP** (5 discovery/proposal tools, ~750 tokens) — Registration, discovery, intent updates, rule proposals
- **REST + CLI** (0 tokens) — All data reads, writes, subscriptions

**Any LLM, any IDE:**

Koor works with Claude Code, Cursor, Kilo Code, or any MCP-compatible client. The REST API works with any HTTP client, making it LLM-agnostic.
