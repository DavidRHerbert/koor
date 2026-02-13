# Deployment

Running Koor in local, LAN, and cloud environments.

## Tier 1: Local (Single Machine)

The simplest setup. No configuration needed.

```bash
./koor-server
```

- API: `localhost:9800`
- Dashboard: `localhost:9847`
- Data: `./data.db`
- Auth: disabled

All agents and tools run on the same machine and connect to `localhost`. This is the default for solo development.

## Tier 2: LAN (Team / Multi-Machine)

Bind to a network interface and enable authentication.

```bash
./koor-server --bind 0.0.0.0:9800 --auth-token secret123
```

Or via environment variables:

```bash
export KOOR_BIND=0.0.0.0:9800
export KOOR_AUTH_TOKEN=secret123
./koor-server
```

Configure agents on other machines:

```bash
koor-cli config set server http://192.168.1.100:9800
koor-cli config set token secret123
```

LAN considerations:
- Bind to `0.0.0.0` to accept connections from any interface
- Always enable auth when binding to a non-localhost address
- The dashboard also binds to its configured address — use `--dashboard-bind 0.0.0.0:9847` for LAN dashboard access
- No TLS on this tier — traffic is unencrypted (suitable for trusted local networks)

## Tier 3: Cloud / Remote

For remote deployments, add TLS termination in front of Koor.

### With a Reverse Proxy (nginx)

```nginx
server {
    listen 443 ssl;
    server_name koor.example.com;

    ssl_certificate /etc/ssl/certs/koor.pem;
    ssl_certificate_key /etc/ssl/private/koor.key;

    location / {
        proxy_pass http://127.0.0.1:9800;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

The `Upgrade` and `Connection` headers are needed for WebSocket event subscriptions.

### Docker

Example `Dockerfile`:

```dockerfile
FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /koor-server ./cmd/koor-server
RUN CGO_ENABLED=0 go build -o /koor-cli ./cmd/koor-cli

FROM alpine:3.19
COPY --from=build /koor-server /usr/local/bin/koor-server
COPY --from=build /koor-cli /usr/local/bin/koor-cli
EXPOSE 9800 9847
VOLUME /data
CMD ["koor-server", "--data-dir", "/data", "--bind", "0.0.0.0:9800", "--dashboard-bind", "0.0.0.0:9847"]
```

Run:

```bash
docker build -t koor .
docker run -d -p 9800:9800 -p 9847:9847 -v koor-data:/data -e KOOR_AUTH_TOKEN=secret123 koor
```

### systemd (Linux)

Create `/etc/systemd/system/koor.service`:

```ini
[Unit]
Description=Koor Coordination Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/koor-server --bind 0.0.0.0:9800 --data-dir /var/lib/koor --auth-token ${AUTH_TOKEN}
EnvironmentFile=/etc/koor/env
Restart=on-failure
RestartSec=5
User=koor
Group=koor

[Install]
WantedBy=multi-user.target
```

Create `/etc/koor/env`:

```
AUTH_TOKEN=your-secret-token
```

Enable and start:

```bash
sudo useradd -r -s /sbin/nologin koor
sudo mkdir -p /var/lib/koor
sudo chown koor:koor /var/lib/koor
sudo systemctl enable koor
sudo systemctl start koor
```

### Windows Service

Use a service wrapper like [NSSM](https://nssm.cc/) or [WinSW](https://github.com/winsw/winsw):

```bash
nssm install Koor "C:\koor\koor-server.exe" "--bind" "0.0.0.0:9800" "--data-dir" "C:\koor\data" "--auth-token" "secret123"
nssm start Koor
```

## Data Backup

The database is a single SQLite file at `{data_dir}/data.db`. To back up:

```bash
# While the server is running (WAL mode supports hot backups):
cp ./data.db ./data.db.backup
cp ./data.db-wal ./data.db-wal.backup
```

Or stop the server and copy `data.db` alone (the WAL is merged on clean shutdown).

## Resource Usage

Koor is lightweight:
- **Memory:** ~10-20 MB idle, scales with concurrent connections
- **Disk:** SQLite database grows with stored state, specs, and event history (pruned to 1000 events)
- **CPU:** Minimal — most operations are simple SQLite reads/writes
- **Connections:** One goroutine per WebSocket subscriber
