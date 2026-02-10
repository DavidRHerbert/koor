# CLI Reference

`koor-cli` is the command-line client for Koor. It communicates with the Koor server via REST API.

## Configuration

The CLI uses a simple TOML-format config file at `~/.koor/config.toml`:

```toml
server = "http://localhost:9800"
token = "my-secret-token"
```

### Setting Configuration

```
koor-cli config set server http://localhost:9800
koor-cli config set token my-secret-token
```

### Priority

Environment variables override the config file:

| Setting | Env Var | Config Key | Default |
|---------|---------|------------|---------|
| Server URL | `KOOR_SERVER` | `server` | `http://localhost:9800` |
| Auth Token | `KOOR_TOKEN` | `token` | *(none)* |

### Global Flags

| Flag | Description |
|------|-------------|
| `--pretty` | Pretty-print JSON output (can be placed anywhere in the command) |

---

## status

Check server health.

```
koor-cli status
```

**Output**

```json
{"status":"ok","uptime":"3h24m10s"}
```

With `--pretty`:

```
koor-cli status --pretty
```

```json
{
  "status": "ok",
  "uptime": "3h24m10s"
}
```

---

## state

Manage shared key/value state.

### state list

List all state keys (summaries, no values).

```
koor-cli state list
```

**Output**

```json
[{"key":"api-contract","version":3,"content_type":"application/json","updated_at":"2026-02-09T14:30:00Z"}]
```

### state get

Get the value for a key.

```
koor-cli state get <key>
```

**Example**

```
koor-cli state get api-contract
```

Returns the raw stored value.

### state set

Set a state value from a file or inline data.

```
koor-cli state set <key> --file <path>
koor-cli state set <key> --data <json>
```

**Examples**

```
koor-cli state set api-contract --data '{"version":"1.0","endpoints":["/api/users"]}'
koor-cli state set build-config --file ./build.json
```

**Output**

```json
{"key":"api-contract","version":1,"hash":"e3b0c44298fc1c14...","content_type":"application/json","updated_at":"2026-02-09T14:30:00Z"}
```

### state delete

Delete a state key.

```
koor-cli state delete <key>
```

**Example**

```
koor-cli state delete api-contract
```

**Output**

```json
{"deleted":"api-contract"}
```

---

## specs

Manage per-project specifications. Spec paths use the format `project/name`.

### specs list

List all specs for a project.

```
koor-cli specs list <project>
```

**Example**

```
koor-cli specs list w2c-forms
```

**Output**

```json
{"project":"w2c-forms","specs":[{"name":"button-schema","version":2,"updated_at":"2026-02-09T14:30:00Z"}]}
```

### specs get

Get a spec's data.

```
koor-cli specs get <project>/<name>
```

**Example**

```
koor-cli specs get w2c-forms/button-schema
```

Returns the raw spec data.

### specs set

Set a spec from a file or inline data.

```
koor-cli specs set <project>/<name> --file <path>
koor-cli specs set <project>/<name> --data <json>
```

**Examples**

```
koor-cli specs set w2c-forms/button-schema --data '{"states":["idle","hover","active"]}'
koor-cli specs set w2c-forms/modal-schema --file ./modal.json
```

**Output**

```json
{"project":"w2c-forms","name":"button-schema","version":1,"hash":"a1b2c3d4...","updated_at":"2026-02-09T14:30:00Z"}
```

### specs delete

Delete a spec.

```
koor-cli specs delete <project>/<name>
```

**Example**

```
koor-cli specs delete w2c-forms/button-schema
```

**Output**

```json
{"deleted":"w2c-forms/button-schema"}
```

---

## events

Publish and subscribe to events.

### events publish

Publish an event to a topic.

```
koor-cli events publish <topic> --data <json>
```

**Example**

```
koor-cli events publish api.change.contract --data '{"version":"2.0","breaking":true}'
```

**Output**

```json
{"id":42,"topic":"api.change.contract","data":{"version":"2.0","breaking":true},"source":"","created_at":"2026-02-09T14:30:00Z"}
```

### events history

Retrieve recent events.

```
koor-cli events history [--last N] [--topic pattern]
```

**Options**

| Flag | Default | Description |
|------|---------|-------------|
| `--last` | `50` | Number of events to return |
| `--topic` | *(all)* | Glob pattern to filter topics |

**Examples**

```
koor-cli events history
koor-cli events history --last 10
koor-cli events history --last 100 --topic "api.*"
```

### events subscribe

Stream events in real-time. Attempts WebSocket connection; falls back to polling history every 2 seconds if no WebSocket client library is available.

```
koor-cli events subscribe [pattern]
```

**Examples**

```
koor-cli events subscribe
koor-cli events subscribe "api.*"
```

The CLI prints a hint about using dedicated WebSocket clients for true real-time streaming:

```
websocat ws://localhost:9800/api/events/subscribe?pattern=api.*
wscat -c ws://localhost:9800/api/events/subscribe?pattern=api.*
```

The polling fallback prints new events as JSON lines to stdout.

---

## register

Register this agent instance with the Koor server.

```
koor-cli register <name> [--workspace <path>] [--intent <text>]
```

**Options**

| Flag | Required | Description |
|------|----------|-------------|
| `<name>` | Yes | Agent name (positional argument) |
| `--workspace` | No | Workspace path or identifier |
| `--intent` | No | Current task description |

**Example**

```
koor-cli register claude-frontend --workspace /projects/frontend --intent "implementing dark mode"
```

**Output** — Returns the full instance including the token (save this):

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "claude-frontend",
  "workspace": "/projects/frontend",
  "intent": "implementing dark mode",
  "token": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "registered_at": "2026-02-09T14:30:00Z",
  "last_seen": "2026-02-09T14:30:00Z"
}
```

---

## instances

Manage registered agent instances.

### instances list

List all registered instances.

```
koor-cli instances list
```

**Output**

```json
[{"id":"550e8400-...","name":"claude-frontend","workspace":"/projects/frontend","intent":"implementing dark mode","registered_at":"2026-02-09T12:00:00Z","last_seen":"2026-02-09T14:30:00Z"}]
```

### instances get

Get details for a specific instance.

```
koor-cli instances get <id>
```

**Example**

```
koor-cli instances get 550e8400-e29b-41d4-a716-446655440000
```

**Output**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "claude-frontend",
  "workspace": "/projects/frontend",
  "intent": "implementing dark mode",
  "registered_at": "2026-02-09T12:00:00Z",
  "last_seen": "2026-02-09T14:30:00Z"
}
```

---

## Full Command Summary

```
koor-cli config set server <url>
koor-cli config set token <token>
koor-cli status

koor-cli state list
koor-cli state get <key>
koor-cli state set <key> --file <path>
koor-cli state set <key> --data <json>
koor-cli state delete <key>

koor-cli specs list <project>
koor-cli specs get <project>/<name>
koor-cli specs set <project>/<name> --file <path>
koor-cli specs set <project>/<name> --data <json>
koor-cli specs delete <project>/<name>

koor-cli events publish <topic> --data <json>
koor-cli events history [--last N] [--topic pattern]
koor-cli events subscribe [pattern]

koor-cli rules import --file <path>
koor-cli rules export [--source <sources>] [--output <path>]

koor-cli register <name> [--workspace <path>] [--intent <text>]
koor-cli instances list
koor-cli instances get <id>
```

---

## Rules Commands

### Import Rules

Import rules from a JSON file. Uses UPSERT, safe to re-run:

```bash
koor-cli rules import --file rules/external/claude-code-rules.json
```

**Output:**

```json
{"imported": 8}
```

### Export Rules

Export rules as JSON. Default exports `local` and `learned` sources (excludes external):

```bash
# Export to stdout
koor-cli rules export

# Export specific sources
koor-cli rules export --source local,learned

# Export to file
koor-cli rules export --output my-org-rules.json

# Export only external rules
koor-cli rules export --source external --output external-backup.json
```
