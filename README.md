# CloudflareSpeedTest Dashboard

Self-hosted dashboard and Linux agent for scheduled HTTPing checks inspired by
[XIU2/CloudflareSpeedTest](https://github.com/XIU2/CloudflareSpeedTest).

## Run locally

```sh
cd web && npm install && npm run build
cd ..
go run ./cmd/server
```

Open `http://127.0.0.1:8080`, create the first admin account, add a host, then
copy the install command from the host detail panel.

## Configuration

Server environment variables:

- `CFST_ADDR`: listen address, default `:8080`.
- `CFST_DB`: SQLite database path, default `cfst-dashboard.db`.
- `DASHBOARD_PUBLIC_URL`: public URL used in copied install commands.

Agent environment variables for first registration:

- `CFST_SERVER`: dashboard URL.
- `CFST_TOKEN`: one-time enrollment token.

For local agent testing:

```sh
CFST_SERVER=http://127.0.0.1:8080 CFST_TOKEN=<token> go run ./cmd/agent
```

After registration, the agent writes its host id and secret to
`~/.cfst-agent.json` for non-root runs or `/opt/cfst-agent/config.json` for root
runs.

## Linux install command

The dashboard serves `/install.sh` and creates a systemd unit at
`/etc/systemd/system/cfst-agent.service`. Build Linux agent binaries before
using copied install commands:

```sh
make dist
```

The install script downloads `/downloads/cfst-agent-linux-amd64` or
`/downloads/cfst-agent-linux-arm64` from the dashboard server. `make dist` also
builds standalone agent binaries for macOS Intel (`cfst-agent-darwin-amd64`),
macOS Apple Silicon (`cfst-agent-darwin-arm64`), Windows Intel
(`cfst-agent-windows-amd64.exe`), and Windows ARM64
(`cfst-agent-windows-arm64.exe`).

## Current MVP scope

- Single admin account created on first startup.
- Agent-initiated polling; no inbound access to agent hosts required.
- SQLite storage with 30-day measurement retention.
- HTTPing latency/status/failure-rate curves per host and target URL.
- Linux systemd target for the install script.
- Per-target schedules; each domain can use a different interval from the dashboard.

## Frontend development

The dashboard UI is a Vite + React + React Router app under `web/`.

```sh
go run ./cmd/server
cd web
npm run dev
```

Vite proxies `/api`, `/install.sh`, and `/downloads` to the Go server.
