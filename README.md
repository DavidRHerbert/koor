# Koor

Lightweight coordination server for AI coding agents — "Redis for AI coding agents."

Koor splits **control** (MCP, thin discovery layer) from **data** (REST + CLI, direct access), so LLMs don't burn tokens routing data through the context window.

## Quickstart

### Build

```bash
go build ./cmd/koor-server
go build ./cmd/koor-cli
```

### Start the server

```bash
./koor-server
# API: localhost:9800  Dashboard: localhost:9847
```

With auth:

```bash
./koor-server --auth-token secret123
```

### Use the CLI

```bash
# Set state
./koor-cli state set api-contract --data '{"version":"1.0"}'

# Get state
./koor-cli state get api-contract

# List state keys
./koor-cli state list

# Check health
./koor-cli status
```

### Configuration

Priority (highest wins): **CLI flags > env vars > config file > defaults**

| Flag | Env Var | Default |
|------|---------|---------|
| `--bind` | `KOOR_BIND` | `localhost:9800` |
| `--dashboard-bind` | `KOOR_DASHBOARD_BIND` | `localhost:9847` |
| `--data-dir` | `KOOR_DATA_DIR` | `~/.koor` |
| `--auth-token` | `KOOR_AUTH_TOKEN` | *(none)* |
| `--log-level` | `KOOR_LOG_LEVEL` | `info` |
| `--config` | — | `./koor.config.json` |

Config file (`koor.config.json`):

```json
{
  "bind": "localhost:9800",
  "dashboard_bind": "localhost:9847",
  "data_dir": "/data/koor",
  "auth_token": "my-secret",
  "log_level": "debug"
}

```

## API Reference

All endpoints except `/health` require `Authorization: Bearer <token>` when auth is enabled.

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Server health and uptime (no auth) |

### State

Key/value store for shared JSON blobs.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/state` | List all keys (summaries) |
| `GET` | `/api/state/{key}` | Get value (supports ETag/If-None-Match) |
| `PUT` | `/api/state/{key}` | Set value (body = raw value) |
| `DELETE` | `/api/state/{key}` | Delete key |

### Specs

Per-project specification storage.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/specs/{project}` | List specs for project |
| `GET` | `/api/specs/{project}/{name}` | Get spec (supports ETag) |
| `PUT` | `/api/specs/{project}/{name}` | Set spec (body = raw data) |
| `DELETE` | `/api/specs/{project}/{name}` | Delete spec |

### Events

Pub/sub with SQLite-backed history and WebSocket streaming.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/events/publish` | Publish event (`{"topic","data","source"}`) |
| `GET` | `/api/events/history` | Event history (`?last=N&topic=pattern`) |
| `GET` | `/api/events/subscribe` | WebSocket stream (`?pattern=topic.*`) |

### Instances

Agent instance registration and discovery.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/instances` | List instances (`?name=&workspace=`) |
| `GET` | `/api/instances/{id}` | Get instance by ID |
| `POST` | `/api/instances/register` | Register (`{"name","workspace","intent"}`) |
| `POST` | `/api/instances/{id}/heartbeat` | Heartbeat (updates last_seen) |
| `DELETE` | `/api/instances/{id}` | Deregister instance |

### Validation

Rule-based content validation.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/validate/{project}/rules` | List rules |
| `PUT` | `/api/validate/{project}/rules` | Set rules (replaces all) |
| `POST` | `/api/validate/{project}` | Validate content against rules |

### MCP

Model Context Protocol endpoint for LLM tool discovery.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/mcp` | StreamableHTTP MCP transport |

**MCP tools:** `register_instance`, `discover_instances`, `set_intent`, `get_endpoints`

### Metrics & Dashboard

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/metrics` | Server metrics (counts) |
| Dashboard | `localhost:9847` | Live web dashboard (separate port) |

## Architecture

```
LLM ──MCP──> koor-server ──> SQLite (WAL mode)
               ▲
Agent ──REST──/
               ▲
CLI ───REST───/
```

- **3 dependencies:** modernc.org/sqlite, nhooyr.io/websocket, mark3labs/mcp-go
- **Pure Go:** CGO_ENABLED=0, cross-compiles to all platforms
- **Single binary:** embed dashboard static files via go:embed

## Development

```bash
go test ./... -v -count=1    # 52 tests
go build ./...               # Build all
```

## License

Private — all rights reserved.
