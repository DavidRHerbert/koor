# Troubleshooting

Common issues and solutions.

## Connection Errors

### "connection refused" when using koor-cli or curl

The server isn't running, or it's on a different address.

**Check:**

```bash
koor-cli status
# or
curl http://localhost:9800/health
```

**Fix:**
- Start the server: `./koor-server`
- If the server is on a different host/port, configure the CLI: `koor-cli config set server http://HOST:PORT`
- Check if the port is already in use: the server logs will show an error on startup

### "invalid or missing bearer token" (401)

Auth is enabled on the server but the client isn't sending a token (or the wrong one).

**Fix:**

```bash
# Set the token in CLI config
koor-cli config set token YOUR_TOKEN

# Or via environment variable
export KOOR_TOKEN=YOUR_TOKEN

# For curl
curl -H "Authorization: Bearer YOUR_TOKEN" http://localhost:9800/api/state
```

## Port Conflicts

### "bind: address already in use"

Another process is using port 9800 or 9847.

**Fix:**
- Use a different port: `./koor-server --bind localhost:9801 --dashboard-bind localhost:9848`
- Find what's using the port:
  - Linux/macOS: `lsof -i :9800`
  - Windows: `netstat -ano | findstr :9800`

## Database Issues

### "failed to open database"

The data directory doesn't exist or isn't writable.

**Fix:**
- Check the data directory: `ls -la .`
- Ensure the current directory is writable
- Check permissions: the server needs read/write access
- Try a different directory: `./koor-server --data-dir /tmp/koor`

### Database locked

Multiple processes trying to write simultaneously beyond the 5-second busy timeout.

**Fix:**
- This is rare with WAL mode. Check for multiple server instances running on the same data directory
- Only one `koor-server` process should use a given data directory at a time

## WebSocket Issues

### WebSocket subscription not receiving events

**Check:**
- Verify the pattern matches your topics. `api.*` matches `api.change` but not `api.change.contract` (single segment match)
- Verify events are being published: `curl http://localhost:9800/api/events/history?last=5`
- Check the server logs for "websocket subscriber connected" and any errors

### "websocket: bad handshake"

The client isn't connecting with the WebSocket protocol.

**Fix:**
- Use a WebSocket client (websocat, wscat), not curl
- If behind a reverse proxy, ensure it passes `Upgrade` and `Connection` headers

### koor-cli subscribe shows "falling back to polling"

This is expected. The CLI doesn't include a WebSocket library (to stay dependency-free). It polls the history endpoint every 2 seconds instead.

For real-time streaming, use a WebSocket client:

```bash
websocat ws://localhost:9800/api/events/subscribe?pattern=*
```

## MCP Issues

### MCP tools not showing up in IDE

**Check:**
- Verify the MCP config URL is correct: `http://localhost:9800/mcp`
- Verify the server is running: `curl http://localhost:9800/health`
- If auth is enabled, ensure the MCP config includes the auth header
- Check your IDE's MCP logs for connection errors

### MCP tools return errors

**Common causes:**
- `register_instance`: Missing required `name` parameter
- `set_intent`: Missing `instance_id` or `intent`
- Auth mismatch: the MCP endpoint requires the same Bearer token as the REST API

## State and Specs

### "key not found" or "spec not found" (404)

The key or spec doesn't exist. State and specs must be created with PUT before they can be read.

**Check what exists:**

```bash
koor-cli state list
koor-cli specs list PROJECT_NAME
```

### Stale data (getting old values)

If using ETag caching (`If-None-Match` header), the `304 Not Modified` response means the value hasn't changed. If you're seeing stale data without ETags, the value simply hasn't been updated.

**Check the version:**

```bash
curl -v http://localhost:9800/api/state/KEY 2>&1 | grep X-Koor-Version
```

## Instance Issues

### Stale instances in the list

Old agent instances that didn't deregister stay in the database.

**Fix:**

```bash
# List instances and check last_seen timestamps
koor-cli instances list --pretty

# Manually deregister stale ones
curl -X DELETE http://localhost:9800/api/instances/INSTANCE_ID
```

There is no automatic stale-instance cleanup in v0.1 — instances persist until explicitly deleted.

## Logging

### Increase log verbosity

```bash
./koor-server --log-level debug
```

Debug level logs include:
- WebSocket write failures
- Detailed request information
- Internal operation details

### Log output

Logs go to stderr in structured text format:

```
time=2026-02-09T14:00:00.000Z level=INFO msg="state updated" key=api-contract version=2
time=2026-02-09T14:00:01.000Z level=INFO msg="event published" topic=api.change.contract id=42
```

## Build Issues

### Build fails with CGO errors

Koor uses `modernc.org/sqlite` (pure Go, no CGO). If you see CGO-related errors, ensure:

```bash
CGO_ENABLED=0 go build ./cmd/koor-server
```

### Tests fail

```bash
go test ./... -v -count=1
```

The `-count=1` flag disables test caching. All 52 tests should pass. If tests fail, check for port conflicts (some tests may use real ports) or file permission issues in the temp directory.
