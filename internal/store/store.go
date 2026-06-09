package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"cfst-dashboard/internal/model"

	_ "github.com/mattn/go-sqlite3"
)

const DefaultIntervalSeconds = model.DefaultIntervalSeconds

type Store struct {
	db *sql.DB
}

type Host = model.Host
type Target = model.Target
type Measurement = model.Measurement
type TopIP = model.TopIP

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	statements := []string{
		`create table if not exists users (id integer primary key autoincrement, username text not null unique, password_hash text not null, created_at datetime not null);`,
		`create table if not exists hosts (id integer primary key autoincrement, name text not null, secret_hash text, agent_version text not null default '', last_seen_at datetime, created_at datetime not null);`,
		`create table if not exists enrollment_tokens (id integer primary key autoincrement, host_id integer not null references hosts(id) on delete cascade, token text not null, token_hash text not null unique, used_at datetime, created_at datetime not null);`,
		`create table if not exists sessions (id integer primary key autoincrement, token_hash text not null, expires_at datetime not null, created_at datetime not null);`,
		`create table if not exists targets (id integer primary key autoincrement, host_id integer not null references hosts(id) on delete cascade, url text not null, interval_seconds integer not null, created_at datetime not null);`,
		`create table if not exists measurements (id integer primary key autoincrement, host_id integer not null references hosts(id) on delete cascade, target_id integer not null references targets(id) on delete cascade, url text not null, checked_at datetime not null, status_code integer not null, latency_ms integer not null, success integer not null, error text not null, colo text not null, failure_rate real not null);`,
		`create index if not exists idx_sessions_expires_at on sessions(expires_at);`,
		`create index if not exists idx_measurements_target_time on measurements(target_id, checked_at);`,
		`create index if not exists idx_targets_host on targets(host_id);`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	if err := s.ensureColumn("measurements", "top_ips", "text not null default '[]'"); err != nil {
		return err
	}
	if err := s.ensureColumn("targets", "disabled", "integer not null default 0"); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureColumn(table, column, definition string) error {
	rows, err := s.db.Query(`pragma table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`alter table ` + table + ` add column ` + column + ` ` + definition)
	return err
}

func (s *Store) SetupNeeded() (bool, error) {
	var count int
	if err := s.db.QueryRow(`select count(*) from users`).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *Store) CreateAdmin(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || len(password) < 8 {
		return errors.New("username is required and password must be at least 8 characters")
	}
	needed, err := s.SetupNeeded()
	if err != nil {
		return err
	}
	if !needed {
		return errors.New("admin already exists")
	}
	_, err = s.db.Exec(`insert into users(username, password_hash, created_at) values(?,?,?)`, username, hashSecret(password), time.Now().UTC())
	return err
}

func (s *Store) Authenticate(username, password string) (bool, error) {
	var hash string
	err := s.db.QueryRow(`select password_hash from users where username = ?`, strings.TrimSpace(username)).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return verifySecret(password, hash), nil
}

func (s *Store) CreateSession(token string, expiresAt time.Time) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("session token is required")
	}
	now := time.Now().UTC()
	if err := s.cleanupExpiredSessions(now); err != nil {
		return err
	}
	_, err := s.db.Exec(`insert into sessions(token_hash, expires_at, created_at) values(?,?,?)`, hashSecret(token), expiresAt.UTC(), now)
	return err
}

func (s *Store) DeleteSession(token string) error {
	rows, err := s.db.Query(`select id, token_hash from sessions`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		var hash string
		if err := rows.Scan(&id, &hash); err != nil {
			_ = rows.Close()
			return err
		}
		if verifySecret(token, hash) {
			ids = append(ids, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.db.Exec(`delete from sessions where id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ValidSession(token string) (bool, error) {
	now := time.Now().UTC()
	if err := s.cleanupExpiredSessions(now); err != nil {
		return false, err
	}
	rows, err := s.db.Query(`select token_hash from sessions where expires_at > ?`, now)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return false, err
		}
		if verifySecret(token, hash) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) cleanupExpiredSessions(now time.Time) error {
	_, err := s.db.Exec(`delete from sessions where expires_at <= ?`, now.UTC())
	return err
}

func (s *Store) CreateHost(name string) (Host, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Host{}, "", errors.New("host name is required")
	}
	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return Host{}, "", err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`insert into hosts(name, created_at) values(?,?)`, name, now)
	if err != nil {
		return Host{}, "", err
	}
	hostID, err := res.LastInsertId()
	if err != nil {
		return Host{}, "", err
	}
	token := randomToken()
	if _, err := tx.Exec(`insert into enrollment_tokens(host_id, token, token_hash, created_at) values(?,?,?,?)`, hostID, token, hashSecret(token), now); err != nil {
		return Host{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return Host{}, "", err
	}
	return Host{ID: hostID, Name: name, Status: "pending", CreatedAt: now, Token: token}, token, nil
}

func (s *Store) ListHosts(publicURL string) ([]Host, error) {
	rows, err := s.db.Query(`select id, name, coalesce(agent_version,''), last_seen_at, created_at from hosts order by created_at desc`)
	if err != nil {
		return nil, err
	}
	hosts := []Host{}
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		h.Status = statusFor(h.LastSeenAt)
		hosts = append(hosts, h)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range hosts {
		token, err := s.EnsureEnrollmentToken(hosts[i].ID)
		if err != nil {
			return nil, err
		}
		hosts[i].InstallCommand = installCommand(publicURL, token)
	}
	return hosts, nil
}

func (s *Store) UpdateHost(id int64, name string, publicURL string) (Host, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Host{}, errors.New("host name is required")
	}
	if _, err := s.db.Exec(`update hosts set name = ? where id = ?`, name, id); err != nil {
		return Host{}, err
	}
	h, err := s.getHost(id)
	if errors.Is(err, sql.ErrNoRows) {
		return Host{}, errors.New("host not found")
	}
	if err != nil {
		return Host{}, err
	}
	h.Status = statusFor(h.LastSeenAt)
	token, err := s.EnsureEnrollmentToken(h.ID)
	if err != nil {
		return Host{}, err
	}
	h.InstallCommand = installCommand(publicURL, token)
	return h, nil
}

func (s *Store) DeleteHost(id int64) error {
	res, err := s.db.Exec(`delete from hosts where id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("host not found")
	}
	return nil
}

func (s *Store) EnsureEnrollmentToken(hostID int64) (string, error) {
	token, err := s.PendingToken(hostID)
	if err != nil {
		return "", err
	}
	if token != "" {
		return token, nil
	}
	var exists int
	if err := s.db.QueryRow(`select count(*) from hosts where id = ?`, hostID).Scan(&exists); err != nil {
		return "", err
	}
	if exists == 0 {
		return "", errors.New("host not found")
	}
	token = randomToken()
	_, err = s.db.Exec(
		`insert into enrollment_tokens(host_id, token, token_hash, created_at) values(?,?,?,?)`,
		hostID,
		token,
		hashSecret(token),
		time.Now().UTC(),
	)
	return token, err
}

func (s *Store) PendingToken(hostID int64) (string, error) {
	var token string
	err := s.db.QueryRow(`select token from enrollment_tokens where host_id = ? and used_at is null order by created_at desc limit 1`, hostID).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return token, err
}

func (s *Store) RegisterAgent(token, hostname, version string) (int64, string, error) {
	rows, err := s.db.Query(`select id, host_id, token_hash, used_at from enrollment_tokens`)
	if err != nil {
		return 0, "", err
	}
	type candidate struct {
		tokenID int64
		hostID  int64
		hash    string
		used    sql.NullTime
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.tokenID, &c.hostID, &c.hash, &c.used); err != nil {
			_ = rows.Close()
			return 0, "", err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Close(); err != nil {
		return 0, "", err
	}
	if err := rows.Err(); err != nil {
		return 0, "", err
	}
	for _, c := range candidates {
		if !c.used.Valid && verifySecret(token, c.hash) {
			secret := randomToken()
			now := time.Now().UTC()
			tx, err := s.db.Begin()
			if err != nil {
				return 0, "", err
			}
			defer tx.Rollback()
			if strings.TrimSpace(hostname) != "" {
				_, err = tx.Exec(`update hosts set name = ?, secret_hash = ?, agent_version = ?, last_seen_at = ? where id = ?`, hostname, hashSecret(secret), version, now, c.hostID)
			} else {
				_, err = tx.Exec(`update hosts set secret_hash = ?, agent_version = ?, last_seen_at = ? where id = ?`, hashSecret(secret), version, now, c.hostID)
			}
			if err != nil {
				return 0, "", err
			}
			if _, err := tx.Exec(`update enrollment_tokens set used_at = ? where id = ?`, now, c.tokenID); err != nil {
				return 0, "", err
			}
			if err := tx.Commit(); err != nil {
				return 0, "", err
			}
			return c.hostID, secret, nil
		}
	}
	return 0, "", errors.New("invalid or used enrollment token")
}

func (s *Store) AuthenticateAgent(hostID int64, secret string) (bool, error) {
	var hash string
	err := s.db.QueryRow(`select coalesce(secret_hash,'') from hosts where id = ?`, hostID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ok := hash != "" && verifySecret(secret, hash)
	if ok {
		_, _ = s.db.Exec(`update hosts set last_seen_at = ? where id = ?`, time.Now().UTC(), hostID)
	}
	return ok, nil
}

func (s *Store) AddTarget(hostID int64, rawURL string, interval int) (Target, error) {
	normalized, err := normalizeURL(rawURL)
	if err != nil {
		return Target{}, err
	}
	interval = normalizeInterval(interval)
	now := time.Now().UTC()
	res, err := s.db.Exec(`insert into targets(host_id, url, interval_seconds, created_at) values(?,?,?,?)`, hostID, normalized, interval, now)
	if err != nil {
		return Target{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Target{}, err
	}
	return Target{ID: id, HostID: hostID, URL: normalized, IntervalSeconds: interval, CreatedAt: now}, nil
}

func (s *Store) ImportTargets(hostID int64, rawURLs []string, interval int) ([]Target, int, error) {
	interval = normalizeInterval(interval)
	existing, err := s.targetURLSet(hostID)
	if err != nil {
		return nil, 0, err
	}
	created := []Target{}
	skipped := 0
	seen := map[string]bool{}
	for _, rawURL := range rawURLs {
		normalized, err := normalizeURL(rawURL)
		if err != nil {
			return nil, skipped, err
		}
		if existing[normalized] || seen[normalized] {
			skipped++
			continue
		}
		target, err := s.AddTarget(hostID, normalized, interval)
		if err != nil {
			return nil, skipped, err
		}
		created = append(created, target)
		seen[normalized] = true
	}
	return created, skipped, nil
}

func (s *Store) UpdateTarget(id int64, rawURL string, interval int, disabled bool) (Target, error) {
	normalized, err := normalizeURL(rawURL)
	if err != nil {
		return Target{}, err
	}
	interval = normalizeInterval(interval)
	if _, err := s.db.Exec(`update targets set url = ?, interval_seconds = ?, disabled = ? where id = ?`, normalized, interval, boolInt(disabled), id); err != nil {
		return Target{}, err
	}
	t, err := s.getTarget(id)
	if errors.Is(err, sql.ErrNoRows) {
		return Target{}, errors.New("target not found")
	}
	return t, err
}

func (s *Store) ListTargets(hostID int64) ([]Target, error) {
	return s.listTargets(hostID, false)
}

func (s *Store) ListEnabledTargets(hostID int64) ([]Target, error) {
	return s.listTargets(hostID, true)
}

func (s *Store) listTargets(hostID int64, enabledOnly bool) ([]Target, error) {
	query := `select id, host_id, url, interval_seconds, disabled, created_at from targets where host_id = ?`
	if enabledOnly {
		query += ` and disabled = 0`
	}
	query += ` order by created_at desc`
	rows, err := s.db.Query(query, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	targets := []Target{}
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func (s *Store) DeleteTarget(id int64) error {
	_, err := s.db.Exec(`delete from targets where id = ?`, id)
	return err
}

func (s *Store) AddMeasurement(m Measurement) error {
	topIPs, err := json.Marshal(m.TopIPs)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`insert into measurements(host_id,target_id,url,checked_at,status_code,latency_ms,success,error,colo,failure_rate,top_ips) values(?,?,?,?,?,?,?,?,?,?,?)`,
		m.HostID, m.TargetID, m.URL, m.CheckedAt, m.StatusCode, m.LatencyMS, boolInt(m.Success), m.Error, m.Colo, m.FailureRate, string(topIPs))
	return err
}

func (s *Store) ListMeasurements(hostID int64, since time.Time) ([]Measurement, error) {
	rows, err := s.db.Query(`select id, host_id, target_id, url, checked_at, status_code, latency_ms, success, error, colo, failure_rate, top_ips from measurements where host_id = ? and checked_at >= ? order by checked_at asc`, hostID, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ms := []Measurement{}
	for rows.Next() {
		var m Measurement
		var success int
		var topIPs string
		if err := rows.Scan(&m.ID, &m.HostID, &m.TargetID, &m.URL, &m.CheckedAt, &m.StatusCode, &m.LatencyMS, &success, &m.Error, &m.Colo, &m.FailureRate, &topIPs); err != nil {
			return nil, err
		}
		m.Success = success == 1
		_ = json.Unmarshal([]byte(topIPs), &m.TopIPs)
		ms = append(ms, m)
	}
	return ms, rows.Err()
}

func (s *Store) Cleanup(retention time.Duration) error {
	_, err := s.db.Exec(`delete from measurements where checked_at < ?`, time.Now().UTC().Add(-retention))
	return err
}

type hostScanner interface {
	Scan(dest ...any) error
}

func scanHost(scanner hostScanner) (Host, error) {
	var h Host
	var lastSeen sql.NullTime
	if err := scanner.Scan(&h.ID, &h.Name, &h.AgentVersion, &lastSeen, &h.CreatedAt); err != nil {
		return h, err
	}
	if lastSeen.Valid {
		h.LastSeenAt = &lastSeen.Time
	}
	return h, nil
}

func (s *Store) getHost(id int64) (Host, error) {
	row := s.db.QueryRow(`select id, name, coalesce(agent_version,''), last_seen_at, created_at from hosts where id = ?`, id)
	return scanHost(row)
}

func (s *Store) getTarget(id int64) (Target, error) {
	row := s.db.QueryRow(`select id, host_id, url, interval_seconds, disabled, created_at from targets where id = ?`, id)
	return scanTarget(row)
}

type targetScanner interface {
	Scan(dest ...any) error
}

func scanTarget(scanner targetScanner) (Target, error) {
	var t Target
	var disabled int
	if err := scanner.Scan(&t.ID, &t.HostID, &t.URL, &t.IntervalSeconds, &disabled, &t.CreatedAt); err != nil {
		return t, err
	}
	t.Disabled = disabled == 1
	return t, nil
}

func (s *Store) targetURLSet(hostID int64) (map[string]bool, error) {
	rows, err := s.db.Query(`select url from targets where host_id = ?`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	urls := map[string]bool{}
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		urls[url] = true
	}
	return urls, rows.Err()
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", errors.New("invalid url")
	}
	return parsed.String(), nil
}

func normalizeInterval(interval int) int {
	if interval <= 0 {
		return DefaultIntervalSeconds
	}
	if interval < 10 {
		return 10
	}
	return interval
}

func installCommand(publicURL, token string) string {
	publicURL = strings.TrimRight(publicURL, "/")
	return fmt.Sprintf("curl -fsSL %s/install.sh | sudo bash -s -- --server %s --token %s", publicURL, publicURL, token)
}

func statusFor(lastSeen *time.Time) string {
	if lastSeen == nil || lastSeen.IsZero() {
		return "pending"
	}
	age := time.Since(*lastSeen)
	if age < 2*time.Minute {
		return "online"
	}
	if age < 15*time.Minute {
		return "stale"
	}
	return "offline"
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashSecret(secret string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	sum := sha256.Sum256(append(salt, []byte(secret)...))
	return base64.RawURLEncoding.EncodeToString(salt) + "." + base64.RawURLEncoding.EncodeToString(sum[:])
}

func verifySecret(secret, encoded string) bool {
	parts := strings.Split(encoded, ".")
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	sum := sha256.Sum256(append(salt, []byte(secret)...))
	return string(sum[:]) == string(expected)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
