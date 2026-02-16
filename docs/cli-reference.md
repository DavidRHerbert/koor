# CLI Reference

`koor-cli` is the command-line client for Koor. It communicates with the Koor server via REST API.

## Configuration

The CLI uses a JSON config file at `./settings.json`:

```json
{
  "server": "http://localhost:9800",
  "token": "my-secret-token"
}
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

### state history

List version history for a state key.

```
koor-cli state history <key> [--limit N]
```

**Options**

| Flag | Default | Description |
|------|---------|-------------|
| `--limit` | `50` | Maximum versions to return |

**Example**

```
koor-cli state history api-contract --limit 10
```

### state rollback

Rollback a state key to a previous version.

```
koor-cli state rollback <key> --version N
```

**Example**

```
koor-cli state rollback api-contract --version 2
```

### state diff

Show the JSON diff between two versions of a state key.

```
koor-cli state diff <key> --v1 N --v2 N
```

**Example**

```
koor-cli state diff api-contract --v1 1 --v2 3
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

Retrieve recent events. Supports time-range and source filtering.

```
koor-cli events history [--last N] [--topic pattern] [--from ISO] [--to ISO] [--source name]
```

**Options**

| Flag | Default | Description |
|------|---------|-------------|
| `--last` | `50` | Number of events to return |
| `--topic` | *(all)* | Glob pattern to filter topics |
| `--from` | *(none)* | Start time (RFC 3339) |
| `--to` | *(none)* | End time (RFC 3339) |
| `--source` | *(none)* | Filter by event source |

**Examples**

```
koor-cli events history
koor-cli events history --last 10
koor-cli events history --last 100 --topic "api.*"
koor-cli events history --from 2026-02-16T14:00:00Z --to 2026-02-16T15:00:00Z
koor-cli events history --source agent-1
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

## activate

Activate an agent instance (confirms CLI connectivity after registration).

```
koor-cli activate <instance-id>
```

**Example**

```
koor-cli activate 550e8400-e29b-41d4-a716-446655440000
```

---

## instances stale

List stale (unresponsive) agents.

```
koor-cli instances stale
```

---

## webhooks

Manage webhook registrations for event notifications.

### webhooks list

```
koor-cli webhooks list
```

### webhooks add

```
koor-cli webhooks add --id <id> --url <url> [--patterns "a.*,b.*"] [--secret <s>]
```

**Options**

| Flag | Required | Description |
|------|----------|-------------|
| `--id` | Yes | Webhook identifier |
| `--url` | Yes | URL to POST events to |
| `--patterns` | No | Comma-separated event patterns (default `*`) |
| `--secret` | No | HMAC signing secret |

**Example**

```
koor-cli webhooks add --id slack-notify --url https://hooks.example.com/koor --patterns "agent.*,compliance.*"
```

### webhooks delete

```
koor-cli webhooks delete <id>
```

### webhooks test

Fire a test event to verify connectivity.

```
koor-cli webhooks test <id>
```

---

## compliance

View and trigger contract compliance checks.

### compliance history

```
koor-cli compliance history [--instance_id <id>] [--limit N]
```

**Options**

| Flag | Default | Description |
|------|---------|-------------|
| `--instance_id` | *(all)* | Filter by agent instance |
| `--limit` | `50` | Maximum results |

### compliance run

Force an immediate compliance check across all active agents.

```
koor-cli compliance run
```

---

## templates

Manage shareable template bundles.

### templates list

```
koor-cli templates list [--kind <k>] [--tag <t>]
```

**Options**

| Flag | Description |
|------|-------------|
| `--kind` | Filter by kind (`rules`, `contracts`, `bundle`) |
| `--tag` | Filter by tag |

### templates get

```
koor-cli templates get <id>
```

### templates create

```
koor-cli templates create --id <id> --name <name> --kind <kind> --file <path> [--tags "a,b"]
```

**Options**

| Flag | Required | Description |
|------|----------|-------------|
| `--id` | Yes | Template identifier |
| `--name` | Yes | Human-readable name |
| `--kind` | Yes | `rules`, `contracts`, or `bundle` |
| `--file` | Yes | Path to JSON data file |
| `--tags` | No | Comma-separated tags |

### templates delete

```
koor-cli templates delete <id>
```

### templates apply

Apply a template to a project.

```
koor-cli templates apply <id> --project <project>
```

---

## audit

Query the immutable audit log.

```
koor-cli audit [--actor <a>] [--action <a>] [--from ISO] [--to ISO] [--limit N]
```

**Options**

| Flag | Default | Description |
|------|---------|-------------|
| `--actor` | *(all)* | Filter by actor |
| `--action` | *(all)* | Filter by action type (e.g. `state.put`) |
| `--from` | *(none)* | Start time (ISO 8601) |
| `--to` | *(none)* | End time (ISO 8601) |
| `--limit` | `50` | Maximum entries |

**Example**

```
koor-cli audit --action state.put --limit 10
koor-cli audit --actor agent-1 --from 2026-02-16T00:00:00Z
```

### audit summary

Aggregated summary of audit activity.

```
koor-cli audit summary [--from ISO] [--to ISO]
```

---

## metrics agents

Query per-agent operational metrics.

```
koor-cli metrics agents [--instance_id <id>] [--period <p>]
koor-cli metrics agents <id> [--period <p>]
```

**Options**

| Flag | Default | Description |
|------|---------|-------------|
| `--instance_id` | *(all)* | Filter by agent instance |
| `--period` | *(all)* | Time period prefix (e.g. `2026-02-16`) |

**Examples**

```
koor-cli metrics agents
koor-cli metrics agents --period 2026-02-16
koor-cli metrics agents 550e8400-e29b-41d4-a716-446655440000
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
koor-cli state history <key> [--limit N]
koor-cli state rollback <key> --version N
koor-cli state diff <key> --v1 N --v2 N

koor-cli specs list <project>
koor-cli specs get <project>/<name>
koor-cli specs set <project>/<name> --file <path>
koor-cli specs set <project>/<name> --data <json>
koor-cli specs delete <project>/<name>

koor-cli events publish <topic> --data <json>
koor-cli events history [--last N] [--topic pattern] [--from ISO] [--to ISO] [--source name]
koor-cli events subscribe [pattern]

koor-cli contract set <project>/<name> --file <path>
koor-cli contract get <project>/<name>
koor-cli contract validate <project>/<name> --endpoint "POST /api/x" --direction request --payload '{...}'
koor-cli contract test <project>/<name> --target http://localhost:8080

koor-cli rules import --file <path>
koor-cli rules export [--source <sources>] [--output <path>]

koor-cli webhooks list
koor-cli webhooks add --id <id> --url <url> [--patterns "a.*,b.*"] [--secret <s>]
koor-cli webhooks delete <id>
koor-cli webhooks test <id>

koor-cli compliance history [--instance_id <id>] [--limit N]
koor-cli compliance run

koor-cli templates list [--kind <k>] [--tag <t>]
koor-cli templates get <id>
koor-cli templates create --id <id> --name <name> --kind <kind> --file <path> [--tags "a,b"]
koor-cli templates delete <id>
koor-cli templates apply <id> --project <project>

koor-cli audit [--actor <a>] [--action <a>] [--from ISO] [--to ISO] [--limit N]
koor-cli audit summary [--from ISO] [--to ISO]

koor-cli metrics agents [--instance_id <id>] [--period <p>]
koor-cli metrics agents <id> [--period <p>]

koor-cli backup --output <path>
koor-cli restore --file <path>

koor-cli register <name> [--workspace <path>] [--intent <text>]
koor-cli activate <instance-id>
koor-cli instances list
koor-cli instances get <id>
koor-cli instances stale
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
