# API Reference

Koor exposes a REST API on the configured bind address (default `localhost:9800`). All responses are JSON. All endpoints except `/health` require authentication when an auth token is configured.

## Authentication

When the server is started with `--auth-token`, every request (except `/health`) must include a Bearer token:

```
Authorization: Bearer <token>
```

If no auth token is configured (local mode), all requests pass through without authentication.

Unauthorized requests return:

```json
{"error": "invalid or missing bearer token", "code": 401}
```

## Error Format

All errors return a JSON body:

```json
{
  "error": "description of what went wrong",
  "code": 404
}
```

Standard HTTP status codes are used: 200 (success), 304 (not modified), 400 (bad request), 401 (unauthorized), 404 (not found), 500 (internal server error).

---

## Health

Health check endpoint. No authentication required.

### GET /health

**Response** `200`

```json
{
  "status": "ok",
  "uptime": "3h24m10s"
}
```

---

## State

Key/value store for shared data. Values can be any content type (defaults to `application/json`). Supports ETag-based caching. Version auto-increments on each update.

### GET /api/state

List all state keys. Returns summaries (no values).

**Response** `200`

```json
[
  {
    "key": "api-contract",
    "version": 3,
    "content_type": "application/json",
    "updated_at": "2026-02-09T14:30:00Z"
  },
  {
    "key": "build-config",
    "version": 1,
    "content_type": "application/json",
    "updated_at": "2026-02-09T12:00:00Z"
  }
]
```

Returns an empty array `[]` when no keys exist.

### GET /api/state/{key...}

Get the value for a key. Keys can contain slashes for project scoping (e.g. `Truck-Wash/backend-task`). Returns the raw stored value with its original content type.

**Response Headers**

| Header | Description |
|--------|-------------|
| `Content-Type` | The content type set when the value was stored |
| `ETag` | SHA-256 hash of the value, quoted (`"abc123..."`) |
| `X-Koor-Version` | Integer version number |

**Response** `200` — Raw value body

**ETag Caching** — Send `If-None-Match` with the ETag value to get a `304 Not Modified` if the value hasn't changed:

```
GET /api/state/api-contract
If-None-Match: "e3b0c44298fc1c149afb..."
```

Response `304` with empty body if unchanged.

**Error** `404`

```json
{"error": "key not found: api-contract", "code": 404}
```

### PUT /api/state/{key...}

Create or update a state entry. Keys can contain slashes for project scoping (e.g. `Truck-Wash/backend-task`). Send the raw value as the request body (up to 10 MB).

**Request Headers**

| Header | Required | Default | Description |
|--------|----------|---------|-------------|
| `Content-Type` | No | `application/json` | Stored with the value |

**Request Body** — Raw value (any format).

**Response** `200`

```json
{
  "key": "api-contract",
  "version": 2,
  "hash": "e3b0c44298fc1c149afb...",
  "content_type": "application/json",
  "updated_at": "2026-02-09T14:30:00Z"
}
```

Version starts at 1 for new keys and increments by 1 on each update.

**Error** `400` — Empty body:

```json
{"error": "empty body", "code": 400}
```

### DELETE /api/state/{key...}

Delete a state entry. Keys can contain slashes for project scoping.

**Response** `200`

```json
{"deleted": "api-contract"}
```

**Error** `404`

```json
{"error": "key not found: api-contract", "code": 404}
```

---

## Specs

Per-project specification storage. Specs are keyed by `{project}/{name}`. Supports ETag caching and auto-incrementing versions.

### GET /api/specs/{project}

List all specs for a project. Returns summaries (no data blobs).

**Response** `200`

```json
{
  "project": "w2c-forms",
  "specs": [
    {
      "name": "button-schema",
      "version": 2,
      "updated_at": "2026-02-09T14:30:00Z"
    },
    {
      "name": "modal-schema",
      "version": 1,
      "updated_at": "2026-02-09T12:00:00Z"
    }
  ]
}
```

Returns `{"project": "...", "specs": []}` when no specs exist for the project.

### GET /api/specs/{project}/{name}

Get a spec's data. Returns the raw stored data.

**Response Headers**

| Header | Description |
|--------|-------------|
| `Content-Type` | `application/json` |
| `ETag` | SHA-256 hash of the data, quoted |
| `X-Koor-Version` | Integer version number |

**Response** `200` — Raw spec data body

**ETag Caching** — Same behaviour as state: send `If-None-Match` for `304 Not Modified`.

**Error** `404`

```json
{"error": "spec not found: w2c-forms/button-schema", "code": 404}
```

### PUT /api/specs/{project}/{name}

Create or update a spec. Send the raw data as the request body (up to 10 MB).

**Response** `200`

```json
{
  "project": "w2c-forms",
  "name": "button-schema",
  "version": 2,
  "hash": "a1b2c3d4e5f6...",
  "updated_at": "2026-02-09T14:30:00Z"
}
```

**Error** `400` — Empty body:

```json
{"error": "empty body", "code": 400}
```

### DELETE /api/specs/{project}/{name}

Delete a spec.

**Response** `200`

```json
{"deleted": "w2c-forms/button-schema"}
```

**Error** `404`

```json
{"error": "spec not found: w2c-forms/button-schema", "code": 404}
```

---

## Events

Pub/sub event bus with SQLite-backed history and real-time WebSocket streaming. Topics are dot-separated strings (e.g. `api.change.contract`). Pattern matching uses glob syntax via `path.Match`.

### POST /api/events/publish

Publish an event to a topic. The event is persisted to history and fanned out to active WebSocket subscribers whose patterns match.

**Request Body**

```json
{
  "topic": "api.change.contract",
  "data": {"version": "2.0", "breaking": true}
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `topic` | Yes | Dot-separated topic string |
| `data` | No | Any JSON value (stored as-is) |

**Response** `200`

```json
{
  "id": 42,
  "topic": "api.change.contract",
  "data": {"version": "2.0", "breaking": true},
  "source": "",
  "created_at": "2026-02-09T14:30:00Z"
}
```

**Error** `400`

```json
{"error": "topic is required", "code": 400}
```

### GET /api/events/history

Retrieve recent events from history.

**Query Parameters**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `last` | `50` | Number of events to return (most recent first) |
| `topic` | `*` (all) | Glob pattern to filter by topic |

**Examples**

```
GET /api/events/history
GET /api/events/history?last=10
GET /api/events/history?last=100&topic=api.*
```

**Response** `200`

```json
[
  {
    "id": 42,
    "topic": "api.change.contract",
    "data": {"version": "2.0"},
    "source": "",
    "created_at": "2026-02-09T14:30:00Z"
  }
]
```

Returns an empty array `[]` when no events match.

### GET /api/events/subscribe

WebSocket endpoint for real-time event streaming. Connect with a WebSocket client to receive events as they are published.

**Query Parameters**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `pattern` | `*` (all) | Glob pattern to filter events by topic |

**Example Connection**

```
ws://localhost:9800/api/events/subscribe?pattern=api.*
```

Each event is sent as a JSON text frame:

```json
{
  "id": 43,
  "topic": "api.change.contract",
  "data": {"version": "2.0"},
  "source": "",
  "created_at": "2026-02-09T14:30:05Z"
}
```

The connection remains open until the client disconnects or the server shuts down. If a subscriber is slow, events may be dropped (64-event channel buffer per subscriber).

**Topic Pattern Matching**

Patterns use Go's `path.Match` glob syntax on dot-separated topics:

| Pattern | Matches |
|---------|---------|
| `*` | All topics |
| `api.*` | `api.change`, `api.deploy` |
| `api.change.*` | `api.change.contract`, `api.change.schema` |

---

## Instances

Agent instance registration and discovery. Each instance gets a unique ID and token on registration. Tokens are only returned on registration, not on subsequent GET requests.

### GET /api/instances

List all registered instances. Supports optional query parameter filters.

**Query Parameters**

| Parameter | Required | Description |
|-----------|----------|-------------|
| `name` | No | Filter by agent name |
| `workspace` | No | Filter by workspace |
| `stack` | No | Filter by technology stack (e.g. `goth`, `react`) |

**Examples**

```
GET /api/instances
GET /api/instances?stack=goth
GET /api/instances?name=claude&workspace=/projects/frontend
```

**Response** `200`

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "claude-frontend",
    "workspace": "/projects/frontend",
    "intent": "implementing dark mode",
    "stack": "goth",
    "registered_at": "2026-02-09T12:00:00Z",
    "last_seen": "2026-02-09T14:30:00Z"
  }
]
```

Returns an empty array `[]` when no instances are registered.

### GET /api/instances/{id}

Get a single instance by ID. Token is not included in the response.

**Response** `200`

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "claude-frontend",
  "workspace": "/projects/frontend",
  "intent": "implementing dark mode",
  "stack": "goth",
  "registered_at": "2026-02-09T12:00:00Z",
  "last_seen": "2026-02-09T14:30:00Z"
}
```

**Error** `404`

```json
{"error": "instance not found: 550e8400-...", "code": 404}
```

### POST /api/instances/register

Register a new agent instance. Returns the instance with its token (save this — it is only returned once).

**Request Body**

```json
{
  "name": "claude-frontend",
  "workspace": "/projects/frontend",
  "intent": "implementing dark mode",
  "stack": "goth"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Agent name (e.g. `claude-frontend`) |
| `workspace` | No | Workspace path or identifier |
| `intent` | No | Current task description |
| `stack` | No | Technology stack identifier (e.g. `goth`, `react`) |

**Response** `200`

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "claude-frontend",
  "workspace": "/projects/frontend",
  "intent": "implementing dark mode",
  "stack": "goth",
  "token": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "registered_at": "2026-02-09T14:30:00Z",
  "last_seen": "2026-02-09T14:30:00Z"
}
```

**Error** `400`

```json
{"error": "name is required", "code": 400}
```

### POST /api/instances/{id}/heartbeat

Update the `last_seen` timestamp for an instance. Call periodically to indicate the agent is still active.

**Request Body** — None required.

**Response** `200`

```json
{"id": "550e8400-...", "status": "ok"}
```

**Error** `404`

```json
{"error": "instance not found: 550e8400-...", "code": 404}
```

### DELETE /api/instances/{id}

Deregister an instance.

**Response** `200`

```json
{"deleted": "550e8400-e29b-41d4-a716-446655440000"}
```

**Error** `404`

```json
{"error": "instance not found: 550e8400-...", "code": 404}
```

---

## Validation

Rule-based content validation. Rules are stored per-project and can check for forbidden patterns (regex), required patterns (missing), or custom checks. Rules can be scoped to a technology stack (e.g. `goth`, `react`) so that stack-specific rules only fire when validating content for that stack.

### GET /api/validate/{project}/rules

List all validation rules for a project. Supports optional stack filter.

**Query Parameters**

| Parameter | Required | Description |
|-----------|----------|-------------|
| `stack` | No | Filter rules by technology stack |

**Examples**

```
GET /api/validate/w2c-forms/rules
GET /api/validate/w2c-forms/rules?stack=goth
```

**Response** `200`

```json
{
  "project": "w2c-forms",
  "rules": [
    {
      "project": "w2c-forms",
      "rule_id": "no-inline-style",
      "severity": "error",
      "match_type": "regex",
      "pattern": "style\\s*=",
      "message": "Inline styles are not allowed",
      "applies_to": ["*.html", "*.templ"],
      "stack": "goth"
    }
  ]
}
```

Returns `{"project": "...", "rules": []}` when no rules exist.

### PUT /api/validate/{project}/rules

Replace all validation rules for a project. Existing rules are deleted and replaced with the provided set.

**Request Body** — Array of rule objects:

```json
[
  {
    "rule_id": "no-inline-style",
    "severity": "error",
    "match_type": "regex",
    "pattern": "style\\s*=",
    "message": "Inline styles are not allowed",
    "applies_to": ["*.html", "*.templ"],
    "stack": "goth"
  },
  {
    "rule_id": "require-data-ai-id",
    "severity": "warning",
    "match_type": "missing",
    "pattern": "data-ai-id",
    "message": "Components should have data-ai-id attributes",
    "applies_to": ["*.templ"]
  }
]
```

**Rule Fields**

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `rule_id` | Yes | — | Unique identifier within the project |
| `severity` | No | `error` | `error` or `warning` |
| `match_type` | No | `regex` | `regex`, `missing`, or `custom` |
| `pattern` | Yes | — | Regex pattern or custom check name |
| `message` | No | Auto-generated | Human-readable violation message |
| `applies_to` | No | `["*"]` | Glob patterns for filename filtering |
| `stack` | No | `""` (all stacks) | Technology stack this rule applies to (e.g. `goth`, `react`). Empty means universal. |

**Match Types**

| Type | Behaviour |
|------|-----------|
| `regex` | Flags each line matching the pattern as a violation |
| `missing` | Flags a violation if the pattern is NOT found anywhere in the content |
| `custom` | Built-in check (currently: `no-console-log`). Unknown custom patterns fall back to regex |

**Response** `200`

```json
{"project": "w2c-forms", "count": 2}
```

### POST /api/validate/{project}

Validate content against all rules for a project.

**Request Body**

```json
{
  "filename": "button.templ",
  "content": "<div style=\"color: red\" class=\"c-button\">\n  <span>Click me</span>\n</div>",
  "stack": "goth"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `filename` | No | Used to match `applies_to` glob patterns. If omitted, all rules run. |
| `content` | Yes | The content to validate |
| `stack` | No | Technology stack to filter rules by. When set, only universal rules (no stack) and rules matching this stack are applied. |

**Response** `200`

```json
{
  "project": "w2c-forms",
  "violations": [
    {
      "rule_id": "no-inline-style",
      "severity": "error",
      "message": "Inline styles are not allowed",
      "line": 1,
      "match": "style=\"color: red\""
    }
  ],
  "count": 1
}
```

Returns `{"project": "...", "violations": [], "count": 0}` when content passes all rules.

---

## Rules Management

Rule lifecycle management — propose, accept, reject, export, and import rules. Rules have three sources (`local`, `learned`, `external`) and a status (`accepted`, `proposed`, `rejected`). Only accepted rules participate in validation.

### POST /api/rules/propose

LLM agents propose a rule after solving an issue. The rule is stored with `source=learned`, `status=proposed` and must be accepted by a user before it fires during validation.

**Request Body**

```json
{
  "project": "w2c-forms",
  "rule_id": "no-hardcoded-colors",
  "severity": "warning",
  "match_type": "regex",
  "pattern": "#[0-9a-fA-F]{3,8}",
  "message": "Use CSS custom properties instead of hardcoded colors",
  "applies_to": ["*.templ", "*.css"],
  "stack": "goth",
  "proposed_by": "550e8400-e29b-41d4-a716-446655440000",
  "context": "Instance found hardcoded hex colors causing theme inconsistency."
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `project` | Yes | Project the rule applies to |
| `rule_id` | Yes | Unique rule identifier |
| `pattern` | Yes | Regex pattern or custom check name |
| `severity` | No | `error` or `warning` (default: `error`) |
| `match_type` | No | `regex`, `missing`, or `custom` (default: `regex`) |
| `message` | No | Human-readable violation message |
| `stack` | No | Technology stack this rule targets |
| `proposed_by` | No | Instance ID of the proposing agent |
| `context` | No | Description of the issue that led to this rule |

**Response** `200`

```json
{"project": "w2c-forms", "rule_id": "no-hardcoded-colors", "status": "proposed"}
```

### POST /api/rules/{project}/{ruleID}/accept

Accept a proposed rule, making it active during validation.

**Response** `200`

```json
{"project": "w2c-forms", "rule_id": "no-hardcoded-colors", "status": "accepted"}
```

**Error** `404` — Rule not found or not in proposed status.

### POST /api/rules/{project}/{ruleID}/reject

Reject a proposed rule. It remains stored but will never fire during validation.

**Response** `200`

```json
{"project": "w2c-forms", "rule_id": "no-hardcoded-colors", "status": "rejected"}
```

**Error** `404` — Rule not found or not in proposed status.

### GET /api/rules/export

Export accepted rules filtered by source. Use this to download your organisation's rules and learned procedures.

**Query Parameters**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `source` | `local,learned` | Comma-separated list of sources to include |

**Examples**

```
GET /api/rules/export
GET /api/rules/export?source=local,learned
GET /api/rules/export?source=external
```

**Response** `200` — Array of rule objects.

```json
[
  {
    "project": "w2c-forms",
    "rule_id": "no-inline-style",
    "severity": "error",
    "match_type": "regex",
    "pattern": "style\\s*=",
    "message": "Inline styles are not allowed",
    "stack": "goth",
    "source": "local",
    "status": "accepted"
  }
]
```

### POST /api/rules/import

Bulk import rules. Uses UPSERT — existing rules with the same `project`/`rule_id` are updated. Imported rules are automatically accepted.

**Request Body** — Array of rule objects:

```json
[
  {
    "project": "w2c-forms",
    "rule_id": "ext-no-console-log",
    "severity": "error",
    "match_type": "regex",
    "pattern": "console\\.log\\(",
    "message": "Remove console.log statements",
    "applies_to": ["*.js", "*.ts"],
    "source": "external"
  }
]
```

**Response** `200`

```json
{"imported": 1}
```

**Error** `400` — Empty rules array.

---

## Metrics

### GET /api/metrics

Server metrics summary.

**Response** `200`

```json
{
  "uptime": "3h24m10s",
  "state_keys": 5,
  "instances": 2,
  "last_event_id": 42,
  "api_bind": "localhost:9800",
  "dashboard_bind": "localhost:9847"
}
```

---

## MCP

Model Context Protocol endpoint using StreamableHTTP transport. This is the discovery-only interface for LLM agents. For data operations, use the REST API or CLI.

### POST /mcp

StreamableHTTP MCP transport. Connect via MCP client libraries (e.g. `mark3labs/mcp-go`, Claude Code MCP config).

**MCP Tools**

| Tool | Parameters | Description |
|------|------------|-------------|
| `register_instance` | `name` (required), `workspace`, `intent`, `stack` | Register this agent instance. Returns instance ID, token, and REST endpoints. |
| `discover_instances` | `name`, `workspace`, `stack` | Discover other registered agent instances. Filters are optional. |
| `set_intent` | `instance_id` (required), `intent` (required) | Update intent and refresh last_seen timestamp. |
| `get_endpoints` | *(none)* | Get all REST API and CLI endpoints for direct data access. |
| `propose_rule` | `project` (required), `rule_id` (required), `pattern` (required), `message` (required), `severity`, `match_type`, `stack`, `proposed_by`, `context` | Propose a validation rule for user review. |

The MCP interface provides 5 lightweight discovery and proposal tools. All data operations (state, specs, events) should go through the REST API directly, bypassing the LLM context window.

---

## Dashboard

The web dashboard runs on a separate port (default `localhost:9847`). It serves embedded static files and proxies `/api/*` requests to the API server to avoid CORS issues.

| Path | Description |
|------|-------------|
| `GET /` | Dashboard web UI |
| `GET /api/*` | Proxied to API server |
| `GET /health` | Health check |
