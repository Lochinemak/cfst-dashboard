package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cfst-dashboard/internal/cfst"
	"cfst-dashboard/internal/model"
)

const version = "0.1.1"

type config struct {
	Server string `json:"server"`
	Token  string `json:"token,omitempty"`
	HostID int64  `json:"host_id,omitempty"`
	Secret string `json:"secret,omitempty"`
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", defaultConfigPath(), "agent config path")
	flag.Parse()

	cfg := config{
		Server: os.Getenv("CFST_SERVER"),
		Token:  os.Getenv("CFST_TOKEN"),
	}
	_ = loadConfig(configPath, &cfg)
	if cfg.Server == "" {
		log.Fatal("CFST_SERVER or config server is required")
	}
	cfg.Server = strings.TrimRight(cfg.Server, "/")

	if cfg.HostID == 0 || cfg.Secret == "" {
		if cfg.Token == "" {
			log.Fatal("CFST_TOKEN is required for first registration")
		}
		registered, err := register(cfg)
		if err != nil {
			log.Fatal(err)
		}
		cfg.HostID = registered.HostID
		cfg.Secret = registered.Secret
		cfg.Token = ""
		if err := saveConfig(configPath, cfg); err != nil {
			log.Printf("could not save config: %v", err)
		}
		log.Printf("registered as host %d", cfg.HostID)
	}

	run(cfg)
}

func run(cfg config) {
	nextRun := map[int64]time.Time{}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := runOnce(cfg, nextRun); err != nil {
			log.Printf("run failed: %v", err)
		}
		<-ticker.C
	}
}

func runOnce(cfg config, nextRun map[int64]time.Time) error {
	tasks, err := fetchTasks(cfg)
	if err != nil {
		return err
	}
	var results []model.Measurement
	now := time.Now()
	active := map[int64]bool{}
	for _, target := range tasks.Targets {
		active[target.ID] = true
		due, ok := nextRun[target.ID]
		if ok && now.Before(due) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		result := cfst.HTTPing(ctx, cfst.HTTPingOptions{URL: target.URL, Attempts: 4, Timeout: 2 * time.Second, UserAgent: target.UserAgent, AcceptAnyStatus: true})
		cancel()
		if !result.Success {
			log.Printf("httping target failed target_id=%d url=%q status=%d successes=%d/%d failure_rate=%.2f colo=%q error=%q top_ips=%d", target.ID, target.URL, result.StatusCode, result.Successes, result.Attempts, result.FailureRate, result.Colo, result.Error, len(result.TopIPs))
		}
		results = append(results, model.Measurement{
			TargetID:    target.ID,
			URL:         target.URL,
			CheckedAt:   result.CheckedAt,
			StatusCode:  result.StatusCode,
			LatencyMS:   result.LatencyMS,
			Success:     result.Success,
			Error:       result.Error,
			Colo:        result.Colo,
			FailureRate: result.FailureRate,
			TopIPs:      mapTopIPs(result.TopIPs),
		})
		nextRun[target.ID] = time.Now().Add(time.Duration(intervalFor(target)) * time.Second)
	}
	for id := range nextRun {
		if !active[id] {
			delete(nextRun, id)
		}
	}
	if len(results) == 0 {
		return nil
	}
	return postResults(cfg, results)
}

func mapTopIPs(items []cfst.TopIP) []model.TopIP {
	if len(items) == 0 {
		return nil
	}
	top := make([]model.TopIP, 0, len(items))
	for _, item := range items {
		top = append(top, model.TopIP{
			IP:         item.IP,
			LatencyMS:  item.LatencyMS,
			StatusCode: item.StatusCode,
			Success:    item.Success,
			Colo:       item.Colo,
		})
	}
	return top
}

func intervalFor(target model.Target) int {
	if target.IntervalSeconds < 10 {
		return 10
	}
	return target.IntervalSeconds
}

type registerResponse struct {
	HostID int64  `json:"host_id"`
	Secret string `json:"secret"`
}

func register(cfg config) (registerResponse, error) {
	hostname, _ := os.Hostname()
	var out registerResponse
	err := postJSON(cfg.Server+"/api/agent/register", nil, map[string]string{
		"token":         cfg.Token,
		"hostname":      hostname,
		"agent_version": version,
	}, &out)
	return out, err
}

type tasksResponse struct {
	Targets []model.Target `json:"targets"`
}

func fetchTasks(cfg config) (tasksResponse, error) {
	var out tasksResponse
	req, err := http.NewRequest(http.MethodGet, cfg.Server+"/api/agent/tasks", nil)
	if err != nil {
		return out, err
	}
	setAgentHeaders(req, cfg)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return out, fmt.Errorf("tasks request failed: %s", resp.Status)
	}
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func postResults(cfg config, results []model.Measurement) error {
	return postJSON(cfg.Server+"/api/agent/results", cfg, map[string]any{"results": results}, nil)
}

func postJSON(url string, cfg any, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c, ok := cfg.(config); ok {
		setAgentHeaders(req, c)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func setAgentHeaders(req *http.Request, cfg config) {
	req.Header.Set("X-Agent-ID", strconv.FormatInt(cfg.HostID, 10))
	req.Header.Set("X-Agent-Secret", cfg.Secret)
}

func defaultConfigPath() string {
	if os.Geteuid() == 0 {
		return "/opt/cfst-agent/config.json"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "cfst-agent.json"
	}
	return filepath.Join(home, ".cfst-agent.json")
}

func loadConfig(path string, cfg *config) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, cfg)
}

func saveConfig(path string, cfg config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
