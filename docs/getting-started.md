# Getting Started

Get Koor running and make your first API call in under 5 minutes.

## Install

### Pre-built Binaries

Download from the [Releases](https://github.com/DavidRHerbert/koor/releases) page. Binaries are available for:

- `koor-server-linux-amd64`
- `koor-server-darwin-amd64` / `koor-server-darwin-arm64`
- `koor-server-windows-amd64.exe`
- `koor-cli-linux-amd64`
- `koor-cli-darwin-amd64` / `koor-cli-darwin-arm64`
- `koor-cli-windows-amd64.exe`

### Build from Source

Requires Go 1.21 or later.

```bash
git clone https://github.com/DavidRHerbert/koor.git
cd koor
go build ./cmd/koor-server
go build ./cmd/koor-cli
```

Both binaries are self-contained with zero external runtime dependencies. The dashboard is embedded in the server binary via `go:embed`.

## Start the Server

```bash
./koor-server
```

Output:

```
time=2026-02-09T14:00:00.000Z level=INFO msg="koor server starting" api=localhost:9800 dashboard=localhost:9847 data_dir=. auth=false
```

The server is now listening on two ports:
- **API:** `localhost:9800` (REST + MCP)
- **Dashboard:** `localhost:9847` (web UI)

Data is stored in `./data.db` (SQLite, WAL mode).

## First Commands

### Using curl

Set a state value:

```bash
curl -X PUT http://localhost:9800/api/state/api-contract \
  -H "Content-Type: application/json" \
  -d '{"version":"1.0","endpoints":["/api/users","/api/orders"]}'
```

Response:

```json
{"key":"api-contract","version":1,"hash":"a1b2c3...","content_type":"application/json","updated_at":"2026-02-09T14:00:01Z"}
```

Get it back:

```bash
curl http://localhost:9800/api/state/api-contract
```

Response:

```json
{"version":"1.0","endpoints":["/api/users","/api/orders"]}
```

List all keys:

```bash
curl http://localhost:9800/api/state
```

Check health:

```bash
curl http://localhost:9800/health
```

### Using koor-cli

Configure the CLI (one-time):

```bash
./koor-cli config set server http://localhost:9800
```

Then use it:

```bash
# Set state
./koor-cli state set api-contract --data '{"version":"1.0","endpoints":["/api/users"]}'

# Get state
./koor-cli state get api-contract

# List keys
./koor-cli state list

# Check health
./koor-cli status

# Pretty-print any output
./koor-cli state list --pretty
```

## Enable Authentication

Start the server with a token:

```bash
./koor-server --auth-token secret123
```

With curl, add the Authorization header:

```bash
curl -H "Authorization: Bearer secret123" http://localhost:9800/api/state
```

With koor-cli, set the token:

```bash
./koor-cli config set token secret123
./koor-cli state list
```

The `/health` endpoint never requires authentication.

## Publish and Subscribe to Events

Publish an event:

```bash
curl -X POST http://localhost:9800/api/events/publish \
  -H "Content-Type: application/json" \
  -d '{"topic":"api.change.contract","data":{"version":"2.0"}}'
```

View recent events:

```bash
curl "http://localhost:9800/api/events/history?last=10"
```

Subscribe to events in real-time (requires a WebSocket client):

```bash
websocat ws://localhost:9800/api/events/subscribe?pattern=api.*
```

Or use the CLI polling fallback:

```bash
./koor-cli events subscribe "api.*"
```

## Register an Agent Instance

```bash
./koor-cli register claude-frontend --workspace /projects/frontend --intent "building login page"
```

Save the returned `token` and `id` — the token is only shown on registration.

List all registered instances:

```bash
./koor-cli instances list --pretty
```

## Open the Dashboard

Navigate to `http://localhost:9847` in a browser. The dashboard shows live state, events, instances, and server metrics.

## Next Steps

- [Multi-Agent Workflow](multi-agent-workflow.md) — Coordinate multiple LLM agents (Controller + Frontend + Backend)
- [Configuration](configuration.md) — All server flags, env vars, and config file options
- [API Reference](api-reference.md) — Complete endpoint documentation
- [CLI Reference](cli-reference.md) — All CLI commands and flags
- [MCP Guide](mcp-guide.md) — Connect LLM agents via MCP
- [Events Guide](events-guide.md) — Pub/sub patterns and WebSocket streaming
- [Specs and Validation](specs-and-validation.md) — Shared specs and content validation
