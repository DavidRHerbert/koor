# Events Guide

Koor's pub/sub event bus with SQLite-backed history and real-time WebSocket streaming.

## Concepts

Events are messages published to a **topic** and stored in an ordered history. Subscribers receive events in real-time via WebSocket. The history is also queryable via REST for agents that weren't connected when an event was published.

### Topics

Topics are dot-separated strings. Examples:

- `api.change.contract`
- `build.started`
- `test.failed.frontend`
- `deploy.production`

There is no topic registration — any string works. Convention is `category.action.detail`.

### Event Structure

Every event has:

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Auto-incrementing ID (unique, ordered) |
| `topic` | string | Dot-separated topic |
| `data` | JSON | Any JSON value (object, array, string, number, null) |
| `source` | string | Source identifier (currently empty, reserved for future use) |
| `created_at` | datetime | When the event was published |

## Publishing Events

### Via REST

```bash
curl -X POST http://localhost:9800/api/events/publish \
  -H "Content-Type: application/json" \
  -d '{"topic":"api.change.contract","data":{"version":"2.0","breaking":true}}'
```

### Via CLI

```bash
koor-cli events publish api.change.contract --data '{"version":"2.0","breaking":true}'
```

### Programmatically

Any HTTP client can POST to `/api/events/publish`:

```json
{
  "topic": "build.completed",
  "data": {
    "project": "frontend",
    "duration_ms": 3200,
    "tests_passed": 47
  }
}
```

## Subscribing to Events

### WebSocket (Real-time)

Connect a WebSocket client to receive events as they are published:

```bash
websocat ws://localhost:9800/api/events/subscribe?pattern=api.*
```

Each event arrives as a JSON text frame:

```json
{"id":42,"topic":"api.change.contract","data":{"version":"2.0"},"source":"","created_at":"2026-02-09T14:30:00Z"}
```

The connection stays open until the client disconnects or the server shuts down.

### CLI Polling Fallback

The CLI doesn't include a WebSocket library (to stay dependency-free), so it falls back to polling the history endpoint every 2 seconds:

```bash
koor-cli events subscribe "api.*"
```

New events are printed as JSON lines to stdout. Previously-seen event IDs are tracked to avoid duplicates.

### Subscriber Buffer

Each WebSocket subscriber has a 64-event channel buffer. If a subscriber is slow and the buffer fills, events are dropped for that subscriber. This prevents one slow subscriber from blocking others.

## Event History

### Query Recent Events

```bash
# Last 50 events (default)
curl http://localhost:9800/api/events/history

# Last 10 events
curl "http://localhost:9800/api/events/history?last=10"

# Filter by topic pattern
curl "http://localhost:9800/api/events/history?last=100&topic=api.*"
```

### Via CLI

```bash
koor-cli events history
koor-cli events history --last 10
koor-cli events history --topic "api.*"
koor-cli events history --last 100 --topic "build.*"
```

## Topic Pattern Matching

Patterns use Go's `path.Match` glob syntax:

| Pattern | Matches | Does Not Match |
|---------|---------|----------------|
| `*` | Everything | — |
| `api.*` | `api.change`, `api.deploy` | `api.change.contract` (only one level) |
| `api.change.*` | `api.change.contract`, `api.change.schema` | `api.deploy` |
| `build.*` | `build.started`, `build.completed` | `test.passed` |

Patterns match against the full topic string. A `*` in a pattern matches any non-dot characters within a single segment.

## Event Pruning

Events are automatically pruned to the most recent 1000. A background goroutine runs every 60 seconds and removes events beyond this limit. This keeps the database from growing indefinitely.

Pruning is based on event ID order — the oldest events are removed first.

## Use Cases

### API Contract Changes

When one agent updates a shared API contract, publish an event so other agents can react:

```bash
# Agent A updates the contract
curl -X PUT http://localhost:9800/api/state/api-contract -d '{"version":"2.0"}'

# Agent A publishes a change event
curl -X POST http://localhost:9800/api/events/publish \
  -d '{"topic":"api.change.contract","data":{"version":"2.0","breaking":true}}'
```

Agent B, subscribed to `api.change.*`, receives the event and re-reads the contract.

### Build/Test Notifications

```bash
# CI publishes build result
curl -X POST http://localhost:9800/api/events/publish \
  -d '{"topic":"build.completed","data":{"status":"success","tests":52}}'
```

### Agent Lifecycle

```bash
# Agent announces it's starting work
curl -X POST http://localhost:9800/api/events/publish \
  -d '{"topic":"agent.started","data":{"name":"claude-frontend","task":"dark mode"}}'

# Agent announces it's done
curl -X POST http://localhost:9800/api/events/publish \
  -d '{"topic":"agent.completed","data":{"name":"claude-frontend","task":"dark mode"}}'
```
