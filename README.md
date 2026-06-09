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

## Dashboard one-click install

On a Linux server with systemd, install or update the dashboard service with:

```sh
curl -fsSL https://raw.githubusercontent.com/Lochinemak/cfst-dashboard/main/scripts/install-dashboard.sh \
  | sudo bash
```

The script builds the Vite dashboard UI, the Go dashboard server, and Linux agent
binaries. It installs the service to `/opt/cfst-dashboard`, stores the SQLite
database under `/var/lib/cfst-dashboard`, and creates
`cfst-dashboard.service`.

The installer supports Linux `amd64` and `arm64`/`aarch64` hosts with systemd,
including Debian, Ubuntu, and CentOS-family systems. If `git`, `go`, `npm`,
`make`, or build tools are missing, it tries to install them with `apt-get`,
`dnf`, or `yum`.

If `CFST_ADDR`, `CFST_DB`, or `DASHBOARD_PUBLIC_URL` are not already set, the
script asks for them interactively. Command-line options override environment
variables.

Useful options:

```sh
sudo scripts/install-dashboard.sh \
  --public-url https://dashboard.example.com \
  --addr :8080 \
  --ref main
```

Non-interactive install:

```sh
curl -fsSL https://raw.githubusercontent.com/Lochinemak/cfst-dashboard/main/scripts/install-dashboard.sh \
  | sudo env CFST_ADDR=:8080 \
      CFST_DB=/var/lib/cfst-dashboard/cfst-dashboard.db \
      DASHBOARD_PUBLIC_URL=https://dashboard.example.com \
      bash
```

If the dashboard server has poor access to GitHub, use a GitHub proxy for both
the initial script download and the repository clone:

```sh
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/Lochinemak/cfst-dashboard/main/scripts/install-dashboard.sh \
  | sudo env GITHUB_PROXY=https://ghfast.top \
      CFST_ADDR=:8080 \
      DASHBOARD_PUBLIC_URL=https://dashboard.example.com \
      bash
```

After installation:

```sh
systemctl status cfst-dashboard
journalctl -u cfst-dashboard -f
```

## Release builds

GitHub Actions builds release artifacts only when a version tag is pushed:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The workflow runs tests, builds `dist/*`, uploads a workflow artifact, creates a
GitHub Release for the tag, and attaches the dashboard and agent binaries as
release assets. Regular pushes to `main` do not run the release build.

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
`/downloads/cfst-agent-linux-arm64` from the dashboard server. The copied agent
install command does not download from GitHub releases; each agent only needs to
reach the dashboard public URL. `make dist` also builds standalone agent
binaries for macOS Intel (`cfst-agent-darwin-amd64`), macOS Apple Silicon
(`cfst-agent-darwin-arm64`), Windows Intel (`cfst-agent-windows-amd64.exe`),
and Windows ARM64 (`cfst-agent-windows-arm64.exe`).

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
