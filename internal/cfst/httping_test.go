package cfst

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPingSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("server", "cloudflare")
		w.Header().Set("cf-ray", "abc-SIN")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := HTTPing(context.Background(), HTTPingOptions{URL: server.URL, Attempts: 2, Timeout: time.Second})
	if !result.Success {
		t.Fatalf("expected success, got error %q", result.Error)
	}
	if result.Successes != 2 {
		t.Fatalf("expected 2 successes, got %d", result.Successes)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
	if result.FailureRate != 0 {
		t.Fatalf("expected zero failure rate, got %f", result.FailureRate)
	}
}

func TestHTTPingUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	result := HTTPing(context.Background(), HTTPingOptions{URL: server.URL, Attempts: 1, Timeout: time.Second})
	if result.Success {
		t.Fatal("expected failure")
	}
	if result.StatusCode != http.StatusTeapot {
		t.Fatalf("expected status 418, got %d", result.StatusCode)
	}
	if result.FailureRate != 1 {
		t.Fatalf("expected full failure rate, got %f", result.FailureRate)
	}
}
