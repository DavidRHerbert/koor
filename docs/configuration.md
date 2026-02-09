# Configuration

Koor server and CLI configuration options, with priority rules and examples.

## Server Configuration

### Priority Order

Configuration is resolved highest-wins:

1. **CLI flags** (highest)
2. **Environment variables**
3. **Config file**
4. **Defaults** (lowest)

If `--config` is provided, the config file is loaded from that path. Otherwise, the server looks for `koor.config.json` in the current directory, then in `~/.koor/koor.config.json`.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--bind` | `localhost:9800` | API listen address (host:port) |
| `--dashboard-bind` | `localhost:9847` | Dashboard listen address (empty string disables dashboard) |
| `--data-dir` | `~/.koor` | Directory for SQLite database and config files |
| `--auth-token` | *(empty)* | Bearer token for API authentication. Empty = no auth (local mode) |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `--config` | *(auto-detected)* | Path to config file. Default: `./koor.config.json` then `~/.koor/koor.config.json` |

### Environment Variables

| Variable | Overrides Flag |
|----------|---------------|
| `KOOR_BIND` | `--bind` |
| `KOOR_DASHBOARD_BIND` | `--dashboard-bind` |
| `KOOR_DATA_DIR` | `--data-dir` |
| `KOOR_AUTH_TOKEN` | `--auth-token` |
| `KOOR_LOG_LEVEL` | `--log-level` |

### Config File

JSON format. All fields are optional — unset fields use their defaults.

**File:** `koor.config.json`

```json
{
  "bind": "localhost:9800",
  "dashboard_bind": "localhost:9847",
  "data_dir": "/data/koor",
  "auth_token": "my-secret-token",
  "log_level": "debug"
}
```

**File locations searched (in order):**

1. `./koor.config.json` (current working directory)
2. `~/.koor/koor.config.json` (data directory)

If `--config path/to/file.json` is provided, that path is used instead.

### Examples

**Local development (defaults):**

```bash
./koor-server
# API: localhost:9800, Dashboard: localhost:9847, No auth, ~/.koor/data.db
```

**LAN access with auth:**

```bash
./koor-server --bind 0.0.0.0:9800 --auth-token secret123
```

**Custom data directory:**

```bash
./koor-server --data-dir /var/lib/koor
```

**Disable dashboard:**

```bash
./koor-server --dashboard-bind ""
```

**Debug logging:**

```bash
./koor-server --log-level debug
```

**Environment variables:**

```bash
export KOOR_BIND=0.0.0.0:9800
export KOOR_AUTH_TOKEN=secret123
export KOOR_LOG_LEVEL=debug
./koor-server
```

---

## CLI Configuration

The CLI stores its config in `~/.koor/config.toml`.

### Config File

```toml
server = "http://localhost:9800"
token = "my-secret-token"
```

### Setting Values

```bash
koor-cli config set server http://192.168.1.100:9800
koor-cli config set token secret123
```

### Environment Variables

| Variable | Overrides | Default |
|----------|-----------|---------|
| `KOOR_SERVER` | `server` config key | `http://localhost:9800` |
| `KOOR_TOKEN` | `token` config key | *(empty)* |

Environment variables take priority over the config file.

### Priority Order

1. **Environment variables** (highest)
2. **Config file** (`~/.koor/config.toml`)
3. **Defaults** (lowest)

---

## Database

Koor uses a single SQLite database at `{data_dir}/data.db`.

- **WAL mode** enabled for concurrent reads during writes
- **Busy timeout:** 5 seconds for write contention
- **Auto-migration:** Tables and indexes are created automatically on first run

### Tables

| Table | Primary Key | Purpose |
|-------|-------------|---------|
| `state` | `key` | Key/value shared state |
| `specs` | `(project, name)` | Per-project specifications |
| `events` | `id` (autoincrement) | Event history |
| `instances` | `id` | Registered agent instances |
| `validation_rules` | `(project, rule_id)` | Content validation rules |

### Indexes

| Index | Column(s) | Purpose |
|-------|-----------|---------|
| `idx_events_topic` | `events.topic` | Fast event filtering by topic |
| `idx_events_created_at` | `events.created_at` | Fast event time queries |
| `idx_instances_last_seen` | `instances.last_seen` | Fast instance liveness queries |

### Event Pruning

Events are automatically pruned to keep the last 1000 entries. The pruning runs every 60 seconds as a background goroutine. This limit is the `maxHistory` parameter in the event bus (set to 1000 at startup).

---

## Server Timeouts

| Timeout | Value | Description |
|---------|-------|-------------|
| Read | 10 seconds | Max time to read request headers and body |
| Write | 30 seconds | Max time to write the response |
| Idle | 60 seconds | Max time for keep-alive connections |
| Shutdown | 5 seconds | Graceful shutdown deadline |

---

## Graceful Shutdown

The server listens for `SIGINT` (Ctrl+C) and `SIGTERM` signals. On receiving either:

1. Stops accepting new connections
2. Waits up to 5 seconds for in-flight requests to complete
3. Shuts down the dashboard server (if running)
4. Shuts down the API server
5. Stops the event pruning goroutine
6. Closes the database connection
