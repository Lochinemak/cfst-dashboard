package cfst

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	coloIATA    = regexp.MustCompile(`[A-Z]{3}`)
	coloCountry = regexp.MustCompile(`[A-Z]{2}`)
	coloGcore   = regexp.MustCompile(`^[a-z]{2}`)
)

const httpingUserAgent = "qBittorrent/4.6.5"

type HTTPingOptions struct {
	URL           string
	Attempts      int
	Timeout       time.Duration
	ValidStatuses []int
	TopN          int
	MaxIPs        int
	Concurrency   int
}

type HTTPingResult struct {
	URL         string    `json:"url"`
	CheckedAt   time.Time `json:"checked_at"`
	StatusCode  int       `json:"status_code"`
	LatencyMS   int64     `json:"latency_ms"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	Colo        string    `json:"colo,omitempty"`
	Attempts    int       `json:"attempts"`
	Successes   int       `json:"successes"`
	FailureRate float64   `json:"failure_rate"`
	TopIPs      []TopIP   `json:"top_ips,omitempty"`
}

type TopIP struct {
	IP         string `json:"ip"`
	LatencyMS  int64  `json:"latency_ms"`
	StatusCode int    `json:"status_code"`
	Success    bool   `json:"success"`
	Colo       string `json:"colo,omitempty"`
	Error      string `json:"error,omitempty"`
}

func HTTPing(ctx context.Context, opts HTTPingOptions) HTTPingResult {
	if opts.Attempts <= 0 {
		opts.Attempts = 4
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Second
	}
	if len(opts.ValidStatuses) == 0 {
		opts.ValidStatuses = []int{200, 301, 302}
	}

	if opts.TopN <= 0 {
		opts.TopN = 5
	}
	if opts.MaxIPs <= 0 {
		opts.MaxIPs = 360
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 80
	}

	result := HTTPingResult{
		URL:       opts.URL,
		CheckedAt: time.Now().UTC(),
		Attempts:  opts.Attempts,
	}
	if shouldScanIPs(opts.URL) {
		top, err := topHTTPingIPs(ctx, opts)
		if err != nil {
			log.Printf("httping Cloudflare IP scan failed url=%q: %v", opts.URL, err)
		} else if len(top) == 0 {
			log.Printf("httping Cloudflare IP scan found no successful candidates url=%q", opts.URL)
		} else {
			result.TopIPs = top
			result.Success = top[0].Success
			result.LatencyMS = top[0].LatencyMS
			result.StatusCode = top[0].StatusCode
			result.Colo = top[0].Colo
			result.Successes = opts.Attempts
			return result
		}
	}
	client := &http.Client{
		Timeout: opts.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()

	var total time.Duration
	var lastErr error
	for i := 0; i < opts.Attempts; i++ {
		start := time.Now()
		status, colo, err := head(ctx, client, opts.URL)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d/%d request failed: %w", i+1, opts.Attempts, err)
			log.Printf("httping attempt failed url=%q attempt=%d/%d error=%v", opts.URL, i+1, opts.Attempts, err)
			continue
		}
		result.StatusCode = status
		if result.Colo == "" {
			result.Colo = colo
		}
		if !validStatus(status, opts.ValidStatuses) {
			lastErr = fmt.Errorf("unexpected status code %d (valid: %s)", status, formatStatuses(opts.ValidStatuses))
			log.Printf("httping attempt returned unexpected status url=%q attempt=%d/%d status=%d valid=%s colo=%q", opts.URL, i+1, opts.Attempts, status, formatStatuses(opts.ValidStatuses), colo)
			continue
		}
		result.Successes++
		total += time.Since(start)
	}

	result.FailureRate = float64(opts.Attempts-result.Successes) / float64(opts.Attempts)
	if result.Successes == 0 {
		if lastErr != nil {
			result.Error = lastErr.Error()
		}
		log.Printf("httping failed url=%q attempts=%d status=%d colo=%q error=%q", opts.URL, opts.Attempts, result.StatusCode, result.Colo, result.Error)
		return result
	}
	result.Success = true
	result.LatencyMS = (total / time.Duration(result.Successes)).Milliseconds()
	return result
}

func shouldScanIPs(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host == "" || host == "localhost" {
		return false
	}
	return net.ParseIP(host) == nil
}

func topHTTPingIPs(ctx context.Context, opts HTTPingOptions) ([]TopIP, error) {
	parsed, err := url.Parse(opts.URL)
	if err != nil || parsed.Host == "" {
		return nil, errors.New("invalid url")
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	ips := sampleCloudflareIPs(opts.MaxIPs)
	if len(ips) == 0 {
		return nil, errors.New("no ip samples")
	}

	jobs := make(chan net.IP, len(ips))
	results := make(chan TopIP, len(ips))
	workers := opts.Concurrency
	if workers > len(ips) {
		workers = len(ips)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				if ctx.Err() != nil {
					return
				}
				results <- httpingIP(ctx, opts, ip.String(), port)
			}
		}()
	}
	for _, ip := range ips {
		jobs <- ip
	}
	close(jobs)
	wg.Wait()
	close(results)

	top := make([]TopIP, 0, opts.TopN)
	successes := 0
	statusCounts := map[int]int{}
	errorCounts := map[string]int{}
	for result := range results {
		if !result.Success {
			if result.StatusCode != 0 {
				statusCounts[result.StatusCode]++
			}
			if result.Error != "" {
				errorCounts[result.Error]++
			}
			continue
		}
		successes++
		top = append(top, result)
	}
	sort.Slice(top, func(i, j int) bool {
		return top[i].LatencyMS < top[j].LatencyMS
	})
	if len(top) > opts.TopN {
		top = top[:opts.TopN]
	}
	if successes == 0 {
		return nil, fmt.Errorf("no successful Cloudflare IP candidates out of %d samples; status_counts=%s request_failures=%s", len(ips), formatStatusCounts(statusCounts), formatErrorCounts(errorCounts))
	}
	log.Printf("httping Cloudflare IP scan completed url=%q samples=%d successes=%d top=%s status_failures=%s request_failures=%s", opts.URL, len(ips), successes, formatTopIPs(top), formatStatusCounts(statusCounts), formatErrorCounts(errorCounts))
	return top, nil
}

func httpingIP(ctx context.Context, opts HTTPingOptions, ip, port string) TopIP {
	client := &http.Client{
		Timeout: opts.Timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				address := net.JoinHostPort(ip, port)
				return (&net.Dialer{Timeout: opts.Timeout}).DialContext(ctx, network, address)
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()

	var total time.Duration
	var status int
	var colo string
	var lastErr error
	successes := 0
	for i := 0; i < opts.Attempts; i++ {
		start := time.Now()
		code, nextColo, err := head(ctx, client, opts.URL)
		if err != nil {
			lastErr = err
			continue
		}
		status = code
		if colo == "" {
			colo = nextColo
		}
		if !validStatus(code, opts.ValidStatuses) {
			lastErr = fmt.Errorf("unexpected status code %d", code)
			continue
		}
		successes++
		total += time.Since(start)
	}
	if successes == 0 {
		result := TopIP{IP: ip, StatusCode: status, Colo: colo}
		if lastErr != nil {
			result.Error = lastErr.Error()
		}
		return result
	}
	return TopIP{
		IP:         ip,
		LatencyMS:  (total / time.Duration(successes)).Milliseconds(),
		StatusCode: status,
		Success:    true,
		Colo:       colo,
	}
}

func sampleCloudflareIPs(maxIPs int) []net.IP {
	ranges := strings.Fields(defaultCloudflareIPv4Ranges)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	ips := make([]net.IP, 0, maxIPs)
	for len(ips) < maxIPs {
		for _, cidr := range ranges {
			if len(ips) >= maxIPs {
				break
			}
			ip, ipNet, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			ones, bits := ipNet.Mask.Size()
			hostBits := bits - ones
			if hostBits <= 0 {
				ips = append(ips, append(net.IP(nil), ip4...))
				continue
			}
			maxHost := uint32(1 << min(hostBits, 24))
			host := uint32(rng.Int31n(int32(maxHost)))
			next := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
			next += host
			candidate := net.IPv4(byte(next>>24), byte(next>>16), byte(next>>8), byte(next))
			if ipNet.Contains(candidate) {
				ips = append(ips, candidate)
			}
		}
		if len(ranges) == 0 {
			break
		}
	}
	return ips
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func formatStatuses(statuses []int) string {
	parts := make([]string, 0, len(statuses))
	for _, status := range statuses {
		parts = append(parts, fmt.Sprint(status))
	}
	return strings.Join(parts, ",")
}

func formatStatusCounts(counts map[int]int) string {
	if len(counts) == 0 {
		return "none"
	}
	statuses := make([]int, 0, len(counts))
	for status := range counts {
		statuses = append(statuses, status)
	}
	sort.Ints(statuses)
	parts := make([]string, 0, len(statuses))
	for _, status := range statuses {
		parts = append(parts, fmt.Sprintf("%d:%d", status, counts[status]))
	}
	return strings.Join(parts, ",")
}

func formatErrorCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "none"
	}
	type item struct {
		message string
		count   int
	}
	items := make([]item, 0, len(counts))
	for message, count := range counts {
		items = append(items, item{message: message, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].message < items[j].message
		}
		return items[i].count > items[j].count
	})
	if len(items) > 5 {
		items = items[:5]
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%q:%d", item.message, item.count))
	}
	return strings.Join(parts, ",")
}

func formatTopIPs(top []TopIP) string {
	if len(top) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(top))
	for _, item := range top {
		parts = append(parts, fmt.Sprintf("%s:%dms/%d/%s", item.IP, item.LatencyMS, item.StatusCode, item.Colo))
	}
	return strings.Join(parts, ",")
}

func head(ctx context.Context, client *http.Client, url string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", httpingUserAgent)
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, headerColo(resp.Header), nil
}

func validStatus(status int, statuses []int) bool {
	for _, candidate := range statuses {
		if status == candidate {
			return true
		}
	}
	return false
}

func headerColo(header http.Header) string {
	if header.Get("server") == "cloudflare" {
		if ray := header.Get("cf-ray"); ray != "" {
			return coloIATA.FindString(ray)
		}
	}
	if header.Get("server") == "CDN77-Turbo" {
		if pop := header.Get("x-77-pop"); pop != "" {
			return coloCountry.FindString(pop)
		}
	}
	if server := header.Get("server"); strings.Contains(server, "BunnyCDN-") {
		return coloCountry.FindString(strings.TrimPrefix(server, "BunnyCDN-"))
	}
	if pop := header.Get("x-amz-cf-pop"); pop != "" {
		return coloIATA.FindString(pop)
	}
	if servedBy := header.Get("x-served-by"); servedBy != "" {
		matches := coloIATA.FindAllString(servedBy, -1)
		if len(matches) > 0 {
			return matches[len(matches)-1]
		}
	}
	if id := header.Get("x-id-fe"); id != "" {
		if match := coloGcore.FindString(id); match != "" {
			return strings.ToUpper(match)
		}
	}
	return ""
}
