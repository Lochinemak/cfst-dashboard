#!/usr/bin/env bash
set -euo pipefail

REPO_URL="https://github.com/Lochinemak/cfst-dashboard.git"
REPO_REF="main"
INSTALL_DIR="/opt/cfst-dashboard"
DATA_DIR="/var/lib/cfst-dashboard"
SERVICE_USER="cfst-dashboard"
ADDR="${CFST_ADDR:-}"
PUBLIC_URL="${DASHBOARD_PUBLIC_URL:-}"
DB_PATH="${CFST_DB:-}"
AUTO_INSTALL_DEPS="yes"
GITHUB_PROXY="${GITHUB_PROXY:-}"

usage() {
  cat <<'EOF'
Usage: install-dashboard.sh [options]

Install or update CloudflareSpeedTest Dashboard as a systemd service.

Options:
  --repo <url>          Git repository URL. Default: https://github.com/Lochinemak/cfst-dashboard.git
  --ref <name>          Git branch, tag, or commit to install. Default: main
  --install-dir <path>  Installation directory. Default: /opt/cfst-dashboard
  --data-dir <path>     Database directory. Default: /var/lib/cfst-dashboard
  --user <name>         System user for the service. Default: cfst-dashboard
  --addr <addr>         Listen address for CFST_ADDR. Default: :8080
  --public-url <url>    Public dashboard URL used in copied agent install commands.
  --db <path>           SQLite database path. Default: <data-dir>/cfst-dashboard.db
  --github-proxy <url>  Prefix GitHub URLs with a proxy, for example https://ghfast.top
  --no-install-deps     Do not install missing packages automatically.
  -h, --help            Show this help.

Environment variables:
  CFST_ADDR, CFST_DB, DASHBOARD_PUBLIC_URL, and GITHUB_PROXY are used when the
  matching command-line option is not provided. Missing values are requested
  interactively when a terminal is available.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo) REPO_URL="$2"; shift 2 ;;
    --ref) REPO_REF="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --user) SERVICE_USER="$2"; shift 2 ;;
    --addr) ADDR="$2"; shift 2 ;;
    --public-url) PUBLIC_URL="$2"; shift 2 ;;
    --db) DB_PATH="$2"; shift 2 ;;
    --github-proxy) GITHUB_PROXY="$2"; shift 2 ;;
    --no-install-deps) AUTO_INSTALL_DEPS="no"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 1 ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "please run as root, for example: curl -fsSL <url> | sudo bash" >&2
  exit 1
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) echo "unsupported Linux architecture: $ARCH. Supported: amd64, arm64/aarch64." >&2; exit 1 ;;
esac

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "unsupported OS: $(uname -s). This installer targets Linux with systemd." >&2
  exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemctl is required. This installer targets systemd-based Linux distributions." >&2
  exit 1
fi

prompt_value() {
  local var_name="$1"
  local prompt="$2"
  local default_value="$3"
  local current_value="${!var_name:-}"
  local input=""

  if [[ -n "$current_value" ]]; then
    return
  fi

  if [[ -r /dev/tty && -w /dev/tty ]]; then
    printf "%s [%s]: " "$prompt" "$default_value" >/dev/tty
    read -r input </dev/tty || true
    if [[ -n "$input" ]]; then
      printf -v "$var_name" '%s' "$input"
    else
      printf -v "$var_name" '%s' "$default_value"
    fi
    return
  fi

  printf -v "$var_name" '%s' "$default_value"
  echo "no interactive terminal available; using $var_name=$default_value"
}

default_public_url() {
  case "$ADDR" in
    :*) echo "http://127.0.0.1$ADDR" ;;
    0.0.0.0:*) echo "http://127.0.0.1:${ADDR##*:}" ;;
    "[::]:"*) echo "http://127.0.0.1:${ADDR##*:}" ;;
    *) echo "http://$ADDR" ;;
  esac
}

proxy_github_url() {
  local url="$1"
  if [[ -z "$GITHUB_PROXY" ]]; then
    echo "$url"
    return
  fi
  case "$url" in
    https://github.com/*|https://raw.githubusercontent.com/*)
      echo "${GITHUB_PROXY%/}/$url"
      ;;
    *)
      echo "$url"
      ;;
  esac
}

prompt_value ADDR "CFST_ADDR listen address" ":8080"
prompt_value DB_PATH "CFST_DB SQLite database path" "$DATA_DIR/cfst-dashboard.db"
prompt_value PUBLIC_URL "DASHBOARD_PUBLIC_URL used by agent install commands" "$(default_public_url)"

echo "install configuration:"
echo "  architecture: linux/$GOARCH"
echo "  CFST_ADDR: $ADDR"
echo "  CFST_DB: $DB_PATH"
echo "  DASHBOARD_PUBLIC_URL: $PUBLIC_URL"
if [[ -n "$GITHUB_PROXY" ]]; then
  echo "  GITHUB_PROXY: $GITHUB_PROXY"
fi
echo "  install dir: $INSTALL_DIR"
echo "  data dir: $DATA_DIR"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

install_missing_dependencies() {
  local missing=()
  local cmd
  for cmd in git go npm make; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      missing+=("$cmd")
    fi
  done
  if ! command -v cc >/dev/null 2>&1 && ! command -v gcc >/dev/null 2>&1; then
    missing+=("gcc")
  fi

  if [[ ${#missing[@]} -eq 0 ]]; then
    return
  fi

  if [[ "$AUTO_INSTALL_DEPS" != "yes" ]]; then
    echo "missing required commands: ${missing[*]}" >&2
    exit 1
  fi

  echo "installing missing build dependencies for Linux/$GOARCH"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y git golang-go nodejs npm make build-essential ca-certificates curl
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y git golang nodejs npm make gcc gcc-c++ ca-certificates curl
  elif command -v yum >/dev/null 2>&1; then
    yum install -y git golang nodejs npm make gcc gcc-c++ ca-certificates curl
  else
    echo "unsupported package manager. Please install: git go npm make gcc ca-certificates curl" >&2
    exit 1
  fi
}

install_missing_dependencies
require_command git
require_command go
require_command npm
require_command make
if ! command -v cc >/dev/null 2>&1 && ! command -v gcc >/dev/null 2>&1; then
  echo "missing required command: cc or gcc" >&2
  exit 1
fi

NOLOGIN_SHELL="/usr/sbin/nologin"
if [[ ! -x "$NOLOGIN_SHELL" && -x /sbin/nologin ]]; then
  NOLOGIN_SHELL="/sbin/nologin"
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

FETCH_REPO_URL="$(proxy_github_url "$REPO_URL")"
echo "fetching $FETCH_REPO_URL ($REPO_REF)"
git clone --depth 1 --branch "$REPO_REF" "$FETCH_REPO_URL" "$TMP_DIR/source" 2>/dev/null || {
  git clone "$FETCH_REPO_URL" "$TMP_DIR/source"
  git -C "$TMP_DIR/source" checkout "$REPO_REF"
}

echo "building dashboard and agent binaries"
(
  cd "$TMP_DIR/source"
  if [[ -f web/package-lock.json ]]; then
    npm --prefix web ci
  else
    npm --prefix web install
  fi
  make dist
)

if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  useradd --system --home "$DATA_DIR" --shell "$NOLOGIN_SHELL" "$SERVICE_USER"
fi

mkdir -p "$INSTALL_DIR" "$INSTALL_DIR/web" "$DATA_DIR"
install -m 0755 "$TMP_DIR/source/dist/cfst-dashboard" "$INSTALL_DIR/cfst-dashboard"
rm -rf "$INSTALL_DIR/web/dist" "$INSTALL_DIR/dist"
cp -R "$TMP_DIR/source/web/dist" "$INSTALL_DIR/web/dist"
cp -R "$TMP_DIR/source/dist" "$INSTALL_DIR/dist"
chown -R "$SERVICE_USER:" "$INSTALL_DIR" "$DATA_DIR"

cat > "$INSTALL_DIR/dashboard.env" <<EOF
CFST_ADDR=$ADDR
CFST_DB=$DB_PATH
EOF

if [[ -n "$PUBLIC_URL" ]]; then
  cat >> "$INSTALL_DIR/dashboard.env" <<EOF
DASHBOARD_PUBLIC_URL=$PUBLIC_URL
EOF
fi

chown "$SERVICE_USER:" "$INSTALL_DIR/dashboard.env"
chmod 0640 "$INSTALL_DIR/dashboard.env"

cat > /etc/systemd/system/cfst-dashboard.service <<EOF
[Unit]
Description=CloudflareSpeedTest Dashboard
After=network-online.target
Wants=network-online.target

[Service]
User=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
EnvironmentFile=$INSTALL_DIR/dashboard.env
ExecStart=$INSTALL_DIR/cfst-dashboard
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable cfst-dashboard
systemctl restart cfst-dashboard

echo "cfst-dashboard service installed or updated"
echo "service: systemctl status cfst-dashboard"
echo "logs:    journalctl -u cfst-dashboard -f"
if [[ -n "$PUBLIC_URL" ]]; then
  echo "open:    $PUBLIC_URL"
elif [[ "$ADDR" == :* ]]; then
  echo "open:    http://127.0.0.1$ADDR"
else
  echo "open:    http://$ADDR"
fi
