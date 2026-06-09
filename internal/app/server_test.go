package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"cfst-dashboard/internal/store"
)

func testHTTPServer(t *testing.T) (*store.Store, *httptest.Server) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	app, err := NewServer(st, "http://example.test")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Handler())
	t.Cleanup(server.Close)
	return st, server
}

func TestSetupLoginAndAgentFlow(t *testing.T) {
	_, server := testHTTPServer(t)
	client := server.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar

	post(t, client, server.URL+"/api/setup", map[string]string{"username": "admin", "password": "password123"}, http.StatusOK, nil)

	var host store.Host
	post(t, client, server.URL+"/api/hosts", map[string]string{"name": "edge-1"}, http.StatusOK, &host)
	if host.Token == "" || host.InstallCommand == "" {
		t.Fatalf("expected token and install command, got %#v", host)
	}

	var reg struct {
		HostID int64  `json:"host_id"`
		Secret string `json:"secret"`
	}
	post(t, client, server.URL+"/api/agent/register", map[string]string{"token": host.Token, "hostname": "edge-1", "agent_version": "test"}, http.StatusOK, &reg)
	post(t, client, server.URL+"/api/agent/register", map[string]string{"token": host.Token, "hostname": "edge-1", "agent_version": "test"}, http.StatusUnauthorized, nil)

	resp, err := client.Get(server.URL + "/api/hosts")
	if err != nil {
		t.Fatal(err)
	}
	var hosts []store.Host
	if err := json.NewDecoder(resp.Body).Decode(&hosts); err != nil {
		_ = resp.Body.Close()
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if len(hosts) != 1 || hosts[0].InstallCommand == "" || hosts[0].Token == host.Token {
		t.Fatalf("expected fresh install command after registration, got %#v", hosts)
	}

	var target store.Target
	post(t, client, server.URL+"/api/hosts/"+strconv.FormatInt(host.ID, 10)+"/targets", map[string]any{"url": "example.com", "user_agent": "curl/8"}, http.StatusOK, &target)
	patch(t, client, server.URL+"/api/targets/"+strconv.FormatInt(target.ID, 10), map[string]any{"url": target.URL, "interval_seconds": 60, "disabled": true, "user_agent": "Transmission/4.0"}, http.StatusOK, &target)
	if target.IntervalSeconds != 60 {
		t.Fatalf("expected updated interval 60, got %d", target.IntervalSeconds)
	}
	if !target.Disabled {
		t.Fatal("expected target disabled")
	}
	if target.UserAgent != "Transmission/4.0" {
		t.Fatalf("expected updated user agent, got %q", target.UserAgent)
	}

	var imported struct {
		Created []store.Target `json:"created"`
		Skipped int            `json:"skipped"`
	}
	post(t, client, server.URL+"/api/hosts/"+strconv.FormatInt(host.ID, 10)+"/targets/import", map[string]any{"text": "example.com\nexample.org\nexample.org"}, http.StatusOK, &imported)
	if len(imported.Created) != 1 || imported.Skipped != 2 {
		t.Fatalf("unexpected import result: %#v", imported)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/agent/tasks", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Agent-ID", strconv.FormatInt(reg.HostID, 10))
	req.Header.Set("X-Agent-Secret", reg.Secret)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected tasks status 200, got %d", resp.StatusCode)
	}
	var tasks struct {
		Targets []store.Target `json:"targets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		_ = resp.Body.Close()
		t.Fatal(err)
	}
	if len(tasks.Targets) != 1 || tasks.Targets[0].URL != "https://example.org" {
		_ = resp.Body.Close()
		t.Fatalf("expected only enabled imported target in agent tasks, got %#v", tasks.Targets)
	}
	_ = resp.Body.Close()

	result := store.Measurement{TargetID: target.ID, URL: target.URL, CheckedAt: time.Now(), StatusCode: 200, LatencyMS: 42, Success: true}
	body := map[string]any{"results": []store.Measurement{result}}
	req = jsonReq(t, http.MethodPost, server.URL+"/api/agent/results", body)
	req.Header.Set("X-Agent-ID", strconv.FormatInt(reg.HostID, 10))
	req.Header.Set("X-Agent-Secret", reg.Secret)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected results status 200, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp, err = client.Get(server.URL + "/api/hosts/" + strconv.FormatInt(host.ID, 10) + "/measurements")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var measurements []store.Measurement
	if err := json.NewDecoder(resp.Body).Decode(&measurements); err != nil {
		t.Fatal(err)
	}
	if len(measurements) != 1 || measurements[0].LatencyMS != 42 {
		t.Fatalf("unexpected measurements: %#v", measurements)
	}

	patch(t, client, server.URL+"/api/hosts/"+strconv.FormatInt(host.ID, 10), map[string]string{"name": "edge-renamed"}, http.StatusOK, &host)
	if host.Name != "edge-renamed" {
		t.Fatalf("expected renamed host, got %#v", host)
	}

	req = jsonReq(t, http.MethodDelete, server.URL+"/api/hosts/"+strconv.FormatInt(host.ID, 10), nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("expected delete status 200, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp, err = client.Get(server.URL + "/api/hosts")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var remainingHosts []store.Host
	if err := json.NewDecoder(resp.Body).Decode(&remainingHosts); err != nil {
		t.Fatal(err)
	}
	if len(remainingHosts) != 0 {
		t.Fatalf("expected deleted host to disappear, got %#v", remainingHosts)
	}
}

func TestSessionSurvivesServerRestart(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	app1, err := NewServer(st, "http://example.test")
	if err != nil {
		t.Fatal(err)
	}
	server1 := httptest.NewServer(app1.Handler())
	client := server1.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar

	post(t, client, server1.URL+"/api/setup", map[string]string{"username": "admin", "password": "password123"}, http.StatusOK, nil)
	server1.Close()

	app2, err := NewServer(st, "http://example.test")
	if err != nil {
		t.Fatal(err)
	}
	server2 := httptest.NewServer(app2.Handler())
	defer server2.Close()

	resp, err := client.Get(server2.URL + "/api/hosts")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected persisted session status 200, got %d", resp.StatusCode)
	}
}

func patch(t *testing.T, client *http.Client, url string, body any, want int, out any) {
	t.Helper()
	resp, err := client.Do(jsonReq(t, http.MethodPatch, url, body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("expected status %d, got %d", want, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}

func post(t *testing.T, client *http.Client, url string, body any, want int, out any) {
	t.Helper()
	resp, err := client.Do(jsonReq(t, http.MethodPost, url, body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("expected status %d, got %d", want, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}

func jsonReq(t *testing.T, method, url string, body any) *http.Request {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}
