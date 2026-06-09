package model

import "time"

const DefaultIntervalSeconds = 300

type Host struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	AgentVersion   string     `json:"agent_version"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	InstallCommand string     `json:"install_command,omitempty"`
	Token          string     `json:"token,omitempty"`
}

type Target struct {
	ID              int64     `json:"id"`
	HostID          int64     `json:"host_id"`
	URL             string    `json:"url"`
	IntervalSeconds int       `json:"interval_seconds"`
	Disabled        bool      `json:"disabled"`
	CreatedAt       time.Time `json:"created_at"`
}

type Measurement struct {
	ID          int64     `json:"id"`
	HostID      int64     `json:"host_id"`
	TargetID    int64     `json:"target_id"`
	URL         string    `json:"url"`
	CheckedAt   time.Time `json:"checked_at"`
	StatusCode  int       `json:"status_code"`
	LatencyMS   int64     `json:"latency_ms"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	Colo        string    `json:"colo,omitempty"`
	FailureRate float64   `json:"failure_rate"`
	TopIPs      []TopIP   `json:"top_ips,omitempty"`
}

type TopIP struct {
	IP         string `json:"ip"`
	LatencyMS  int64  `json:"latency_ms"`
	StatusCode int    `json:"status_code"`
	Success    bool   `json:"success"`
	Colo       string `json:"colo,omitempty"`
}
