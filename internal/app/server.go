package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cfst-dashboard/internal/store"
)

type Server struct {
	store     *store.Store
	publicURL string
	staticDir string
}

func NewServer(st *store.Store, publicURL string) (*Server, error) {
	if publicURL == "" {
		publicURL = "http://127.0.0.1:8080"
	}
	staticDir := "web/dist"
	if _, err := os.Stat(filepath.Join(staticDir, "index.html")); err != nil {
		staticDir = "web/static"
	}
	return &Server{
		store:     st,
		publicURL: strings.TrimRight(publicURL, "/"),
		staticDir: staticDir,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/setup/status", s.handleSetupStatus)
	mux.HandleFunc("/api/setup", s.handleSetup)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.withAuth(s.handleLogout))
	mux.HandleFunc("/api/hosts", s.withAuth(s.handleHosts))
	mux.HandleFunc("/api/hosts/", s.withAuth(s.handleHostSubroutes))
	mux.HandleFunc("/api/targets/", s.withAuth(s.handleTargetSubroutes))
	mux.HandleFunc("/api/agent/register", s.handleAgentRegister)
	mux.HandleFunc("/api/agent/tasks", s.withAgent(s.handleAgentTasks))
	mux.HandleFunc("/api/agent/results", s.withAgent(s.handleAgentResults))
	mux.Handle("/downloads/", http.StripPrefix("/downloads/", http.FileServer(http.Dir("dist"))))
	mux.HandleFunc("/install.sh", s.handleInstallScript)
	mux.HandleFunc("/", s.handleStatic)
	return logging(mux)
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	needed, err := s.store.SetupNeeded()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"setup_required": needed})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.CreateAdmin(req.Username, req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.issueSession(w); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ok, err := s.store.Authenticate(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, errors.New("invalid username or password"))
		return
	}
	if err := s.issueSession(w); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("cfst_session"); err == nil {
		if err := s.store.DeleteSession(cookie.Value); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "cfst_session", Path: "/", MaxAge: -1})
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleHosts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		hosts, err := s.store.ListHosts(s.publicURL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, hosts)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		host, token, err := s.store.CreateHost(req.Name)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		host.InstallCommand = fmt.Sprintf("curl -fsSL %s/install.sh | sudo bash -s -- --server %s --token %s", s.publicURL, s.publicURL, token)
		writeJSON(w, host)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleHostSubroutes(w http.ResponseWriter, r *http.Request) {
	id, rest, ok := parseIDRoute("/api/hosts/", r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if rest == "" {
		switch r.Method {
		case http.MethodPatch:
			var req struct {
				Name string `json:"name"`
			}
			if err := readJSON(r, &req); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			host, err := s.store.UpdateHost(id, req.Name, s.publicURL)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, host)
		case http.MethodDelete:
			if err := s.store.DeleteHost(id); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		default:
			methodNotAllowed(w)
		}
		return
	}
	switch rest {
	case "/targets":
		if r.Method == http.MethodGet {
			targets, err := s.store.ListTargets(id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, targets)
			return
		}
		if r.Method == http.MethodPost {
			var req struct {
				URL             string `json:"url"`
				IntervalSeconds int    `json:"interval_seconds"`
			}
			if err := readJSON(r, &req); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			target, err := s.store.AddTarget(id, req.URL, req.IntervalSeconds)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, target)
			return
		}
	case "/targets/import":
		if r.Method == http.MethodPost {
			var req struct {
				Text            string   `json:"text"`
				URLs            []string `json:"urls"`
				IntervalSeconds int      `json:"interval_seconds"`
			}
			if err := readJSON(r, &req); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			urls := req.URLs
			if req.Text != "" {
				urls = append(urls, splitImportText(req.Text)...)
			}
			targets, skipped, err := s.store.ImportTargets(id, urls, req.IntervalSeconds)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, map[string]any{"created": targets, "skipped": skipped})
			return
		}
	case "/measurements":
		since := time.Now().UTC().Add(-30 * 24 * time.Hour)
		if raw := r.URL.Query().Get("since"); raw != "" {
			if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
				since = parsed
			}
		}
		measurements, err := s.store.ListMeasurements(id, since)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, measurements)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleTargetSubroutes(w http.ResponseWriter, r *http.Request) {
	id, rest, ok := parseIDRoute("/api/targets/", r.URL.Path)
	if !ok || rest != "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			URL             string `json:"url"`
			IntervalSeconds int    `json:"interval_seconds"`
			Disabled        bool   `json:"disabled"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		target, err := s.store.UpdateTarget(id, req.URL, req.IntervalSeconds, req.Disabled)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, target)
	case http.MethodDelete:
		if err := s.store.DeleteTarget(id); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Token        string `json:"token"`
		Hostname     string `json:"hostname"`
		AgentVersion string `json:"agent_version"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	hostID, secret, err := s.store.RegisterAgent(req.Token, req.Hostname, req.AgentVersion)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, map[string]any{"host_id": hostID, "secret": secret, "interval_seconds": store.DefaultIntervalSeconds})
}

func (s *Server) handleAgentTasks(w http.ResponseWriter, r *http.Request) {
	hostID := agentHostID(r)
	targets, err := s.store.ListEnabledTargets(hostID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"targets": targets})
}

func splitImportText(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';' || r == '\t' || r == ' '
	})
}

func (s *Server) handleAgentResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	hostID := agentHostID(r)
	var req struct {
		Results []store.Measurement `json:"results"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	for _, result := range req.Results {
		result.HostID = hostID
		if result.CheckedAt.IsZero() {
			result.CheckedAt = time.Now().UTC()
		}
		if err := s.store.AddMeasurement(result); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = w.Write([]byte(`#!/usr/bin/env bash
set -euo pipefail

SERVER=""
TOKEN=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --server) SERVER="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

if [[ -z "$SERVER" || -z "$TOKEN" ]]; then
  echo "usage: install.sh --server <url> --token <token>" >&2
  exit 1
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

INSTALL_DIR="/opt/cfst-agent"
mkdir -p "$INSTALL_DIR"
TMP_AGENT="$INSTALL_DIR/cfst-agent.$$.new"
trap 'rm -f "$TMP_AGENT"' EXIT
curl -fsSL "$SERVER/downloads/cfst-agent-linux-$GOARCH" -o "$TMP_AGENT"
install -m 0755 "$TMP_AGENT" "$INSTALL_DIR/cfst-agent"
cat > "$INSTALL_DIR/agent.env" <<EOF
CFST_SERVER=$SERVER
CFST_TOKEN=$TOKEN
EOF
rm -f "$INSTALL_DIR/config.json"
cat > /etc/systemd/system/cfst-agent.service <<'EOF'
[Unit]
Description=CloudflareSpeedTest Dashboard Agent
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/opt/cfst-agent/agent.env
ExecStart=/opt/cfst-agent/cfst-agent
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable cfst-agent
systemctl restart cfst-agent
echo "cfst-agent service installed or updated"
`))
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	requestPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if requestPath == "." {
		requestPath = "index.html"
	}
	fullPath := filepath.Join(s.staticDir, requestPath)
	if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, fullPath)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.staticDir, "index.html"))
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		needed, err := s.store.SetupNeeded()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if needed {
			writeError(w, http.StatusUnauthorized, errors.New("setup required"))
			return
		}
		cookie, err := r.Cookie("cfst_session")
		if err != nil {
			writeError(w, http.StatusUnauthorized, errors.New("login required"))
			return
		}
		ok, err := s.store.ValidSession(cookie.Value)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, errors.New("login required"))
			return
		}
		next(w, r)
	}
}

func (s *Server) withAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hostID, err := strconv.ParseInt(r.Header.Get("X-Agent-ID"), 10, 64)
		if err != nil || hostID <= 0 {
			writeError(w, http.StatusUnauthorized, errors.New("missing agent id"))
			return
		}
		ok, err := s.store.AuthenticateAgent(hostID, r.Header.Get("X-Agent-Secret"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, errors.New("invalid agent credentials"))
			return
		}
		ctx := context.WithValue(r.Context(), agentHostKey{}, hostID)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) issueSession(w http.ResponseWriter) error {
	token := randomSession()
	expiresAt := time.Now().Add(24 * time.Hour)
	if err := s.store.CreateSession(token, expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "cfst_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400})
	return nil
}

type agentHostKey struct{}

func agentHostID(r *http.Request) int64 {
	id, _ := r.Context().Value(agentHostKey{}).(int64)
	return id
}

func parseIDRoute(prefix, path string) (int64, string, bool) {
	trimmed := strings.TrimPrefix(path, prefix)
	if trimmed == path || trimmed == "" {
		return 0, "", false
	}
	parts := strings.SplitN(trimmed, "/", 2)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", false
	}
	rest := ""
	if len(parts) == 2 {
		rest = "/" + parts[1]
	}
	return id, rest, true
}

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(out)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func randomSession() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
