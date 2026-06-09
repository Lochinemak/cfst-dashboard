package store

import (
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSetupAndAuthenticate(t *testing.T) {
	st := testStore(t)
	needed, err := st.SetupNeeded()
	if err != nil {
		t.Fatal(err)
	}
	if !needed {
		t.Fatal("expected setup to be needed")
	}
	if err := st.CreateAdmin("admin", "password123"); err != nil {
		t.Fatal(err)
	}
	ok, err := st.Authenticate("admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected auth success")
	}
}

func TestSessionPersistsUntilDeletedOrExpired(t *testing.T) {
	st := testStore(t)
	token := "session-token"
	if err := st.CreateSession(token, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ok, err := st.ValidSession(token)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected session to be valid")
	}
	ok, err = st.ValidSession("wrong-token")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected wrong token to be invalid")
	}
	if err := st.DeleteSession(token); err != nil {
		t.Fatal(err)
	}
	ok, err = st.ValidSession(token)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected deleted session to be invalid")
	}
	if err := st.CreateSession("expired-token", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	ok, err = st.ValidSession("expired-token")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected expired session to be invalid")
	}
}

func TestEnrollmentTokenCanBeUsedOnce(t *testing.T) {
	st := testStore(t)
	host, token, err := st.CreateHost("edge-1")
	if err != nil {
		t.Fatal(err)
	}
	hostID, secret, err := st.RegisterAgent(token, "edge-1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if hostID != host.ID || secret == "" {
		t.Fatalf("unexpected registration: hostID=%d secret=%q", hostID, secret)
	}
	if _, _, err := st.RegisterAgent(token, "edge-1", "test"); err == nil {
		t.Fatal("expected reused token to fail")
	}
	ok, err := st.AuthenticateAgent(hostID, secret)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected agent auth success")
	}
	nextToken, err := st.EnsureEnrollmentToken(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nextToken == "" || nextToken == token {
		t.Fatalf("expected a fresh enrollment token, got %q", nextToken)
	}
}

func TestUpdateAndDeleteHost(t *testing.T) {
	st := testStore(t)
	host, _, err := st.CreateHost("edge-1")
	if err != nil {
		t.Fatal(err)
	}
	target, err := st.AddTarget(host.ID, "example.com", 300)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddMeasurement(Measurement{HostID: host.ID, TargetID: target.ID, URL: target.URL, CheckedAt: time.Now(), Success: true}); err != nil {
		t.Fatal(err)
	}

	renamed, err := st.UpdateHost(host.ID, "edge-renamed", "http://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "edge-renamed" {
		t.Fatalf("expected renamed host, got %#v", renamed)
	}
	if renamed.InstallCommand == "" {
		t.Fatal("expected install command after host update")
	}

	if err := st.DeleteHost(host.ID); err != nil {
		t.Fatal(err)
	}
	targets, err := st.ListTargets(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("expected host targets to be deleted, got %#v", targets)
	}
	measurements, err := st.ListMeasurements(host.ID, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(measurements) != 0 {
		t.Fatalf("expected host measurements to be deleted, got %#v", measurements)
	}
	if _, err := st.UpdateHost(host.ID, "missing", "http://example.test"); err == nil {
		t.Fatal("expected updating deleted host to fail")
	}
}

func TestCleanupRemovesOldMeasurements(t *testing.T) {
	st := testStore(t)
	host, _, err := st.CreateHost("edge-1")
	if err != nil {
		t.Fatal(err)
	}
	target, err := st.AddTarget(host.ID, "example.com", 300)
	if err != nil {
		t.Fatal(err)
	}
	old := Measurement{HostID: host.ID, TargetID: target.ID, URL: target.URL, CheckedAt: time.Now().Add(-48 * time.Hour), Success: true}
	recent := Measurement{HostID: host.ID, TargetID: target.ID, URL: target.URL, CheckedAt: time.Now(), Success: true, TopIPs: []TopIP{{IP: "104.16.1.1", LatencyMS: 42, StatusCode: 200, Success: true}}}
	if err := st.AddMeasurement(old); err != nil {
		t.Fatal(err)
	}
	if err := st.AddMeasurement(recent); err != nil {
		t.Fatal(err)
	}
	if err := st.Cleanup(24 * time.Hour); err != nil {
		t.Fatal(err)
	}
	got, err := st.ListMeasurements(host.ID, time.Now().Add(-72*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 measurement after cleanup, got %d", len(got))
	}
	if len(got[0].TopIPs) != 1 || got[0].TopIPs[0].IP != "104.16.1.1" {
		t.Fatalf("expected top ips to round trip, got %#v", got[0].TopIPs)
	}
}

func TestUpdateTargetInterval(t *testing.T) {
	st := testStore(t)
	host, _, err := st.CreateHost("edge-1")
	if err != nil {
		t.Fatal(err)
	}
	target, err := st.AddTarget(host.ID, "example.com", 300)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := st.UpdateTarget(target.ID, "https://example.com/health", 60, true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.URL != "https://example.com/health" {
		t.Fatalf("unexpected url %q", updated.URL)
	}
	if updated.IntervalSeconds != 60 {
		t.Fatalf("unexpected interval %d", updated.IntervalSeconds)
	}
	if !updated.Disabled {
		t.Fatal("expected target to be disabled")
	}
	enabled, err := st.ListEnabledTargets(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 0 {
		t.Fatalf("expected disabled target to be excluded, got %#v", enabled)
	}
}

func TestImportTargetsSkipsExisting(t *testing.T) {
	st := testStore(t)
	host, _, err := st.CreateHost("edge-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddTarget(host.ID, "example.com", 300); err != nil {
		t.Fatal(err)
	}
	created, skipped, err := st.ImportTargets(host.ID, []string{"example.com", "https://example.org", "https://example.org"}, 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || skipped != 2 {
		t.Fatalf("expected 1 created and 2 skipped, got created=%#v skipped=%d", created, skipped)
	}
}
