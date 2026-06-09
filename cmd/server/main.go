package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"cfst-dashboard/internal/app"
	"cfst-dashboard/internal/store"
)

func main() {
	addr := env("CFST_ADDR", ":8080")
	dbPath := env("CFST_DB", "cfst-dashboard.db")
	publicURL := os.Getenv("DASHBOARD_PUBLIC_URL")
	if publicURL == "" {
		publicURL = "http://127.0.0.1" + portSuffix(addr)
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := st.Cleanup(30 * 24 * time.Hour); err != nil {
				log.Printf("retention cleanup failed: %v", err)
			}
		}
	}()

	srv, err := app.NewServer(st, publicURL)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("cfst dashboard listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func portSuffix(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[idx:]
	}
	return ":8080"
}
