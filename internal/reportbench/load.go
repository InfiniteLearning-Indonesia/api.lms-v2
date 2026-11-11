// Package reportbench measures only bounded, explicitly synthetic loopback APIs.
package reportbench

import (
	"context"
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	BaseURL         string `json:"base_url"`
	Session         string `json:"session"`
	CSRF            string `json:"csrf"`
	ClassID         string `json:"class_id"`
	LearnerID       string `json:"learner_id"`
	Kind            string `json:"kind"`
	Requests        int    `json:"requests"`
	Concurrency     int    `json:"concurrency"`
	Warmups         int    `json:"warmups"`
	ExpectedItems   int    `json:"expected_items"`
	ExpectedVersion int    `json:"expected_version"`
}

var hex32 = regexp.MustCompile(`^[a-f0-9]{32}$`)
var hex64 = regexp.MustCompile(`^[a-f0-9]{64}$`)
var errConfig = errors.New("SYNTHETIC_LOAD_CONFIG_REJECTED")

func Decode(r io.Reader) (Config, error) {
	b, e := io.ReadAll(io.LimitReader(r, 4097))
	if e != nil || len(b) > 4096 {
		return Config{}, errConfig
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	var c Config
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || c.Validate() != nil {
		return Config{}, errConfig
	}
	return c, nil
}
func (c Config) Validate() error {
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Scheme != "https" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return errConfig
	}
	p, e := strconv.Atoi(u.Port())
	if e != nil || p < 1 || p > 65535 {
		return errConfig
	}
	if !hex32.MatchString(c.ClassID) || !hex32.MatchString(c.LearnerID) || !hex64.MatchString(c.Session) || !hex64.MatchString(c.CSRF) {
		return errConfig
	}
	if c.Requests < 1 || c.Requests > 30 || (c.Concurrency != 1 && c.Concurrency != 4) || c.Warmups != 2 || (c.ExpectedItems != 2 && c.ExpectedItems != 200) || (c.ExpectedVersion != 1 && c.ExpectedVersion != 3) {
		return errConfig
	}
	if c.Kind != "report" && c.Kind != "page" && c.Kind != "export" {
		return errConfig
	}
	return nil
}

type Result struct {
	Kind         string         `json:"kind"`
	Items        int            `json:"items"`
	Version      int            `json:"grade_version"`
	Concurrency  int            `json:"concurrency"`
	Requests     int            `json:"requests"`
	Warmups      int            `json:"warmups"`
	Errors       int            `json:"errors"`
	WarmupErrors int            `json:"warmup_errors"`
	ErrorCodes   map[string]int `json:"error_codes"`
	Latencies    []float64      `json:"latencies_ms"`
	DurationMS   float64        `json:"duration_ms"`
	Throughput   float64        `json:"requests_per_second"`
	P50          float64        `json:"p50_ms"`
	P95          float64        `json:"p95_ms"`
	P99          float64        `json:"p99_ms"`
	MaxBytes     int            `json:"max_response_bytes"`
	OS           string         `json:"os"`
	Arch         string         `json:"arch"`
	GoVersion    string         `json:"go_version"`
	CPUs         int            `json:"cpus"`
}

func Percentile(values []float64, p float64) float64 {
	if len(values) == 0 || p <= 0 || p > 1 {
		return 0
	}
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	return v[int(math.Ceil(float64(len(v))*p))-1]
}

func contract(c Config, response *http.Response, b []byte) bool {
	if response.StatusCode != 200 || response.Header.Get("Cache-Control") != "no-store" {
		return false
	}
	if c.Kind == "export" {
		if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/csv") || !strings.Contains(response.Header.Get("Content-Disposition"), "attachment") || !hex64.MatchString(response.Header.Get("X-Export-Audit-Id")) || !hex64.MatchString(response.Header.Get("X-Report-Snapshot")) {
			return false
		}
		rows, e := csv.NewReader(strings.NewReader(string(b))).ReadAll()
		if e != nil || len(rows) != c.ExpectedItems+1 || strings.Join(rows[0], ",") != "class_id,learner_user_id,grade_item_id,title,value_milli,grade_version,release_id,released_at" {
			return false
		}
		seen := map[string]bool{}
		for _, row := range rows[1:] {
			if len(row) != 8 || row[0] != c.ClassID || row[1] != c.LearnerID || !hex32.MatchString(row[2]) || seen[row[2]] || row[5] != strconv.Itoa(c.ExpectedVersion) || !hex32.MatchString(row[6]) {
				return false
			}
			seen[row[2]] = true
		}
		return true
	}
	var v struct {
		Version  int    `json:"contract_version"`
		Class    string `json:"class_id"`
		Learner  string `json:"learner_user_id"`
		Snapshot string `json:"snapshot"`
		Cursor   string `json:"next_cursor"`
		Grades   []struct {
			ID      string `json:"grade_item_id"`
			Version int    `json:"grade_version"`
			Release string `json:"release_id"`
		} `json:"grades"`
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") || json.Unmarshal(b, &v) != nil || v.Version != 1 || v.Class != c.ClassID || v.Learner != c.LearnerID {
		return false
	}
	n := c.ExpectedItems
	if c.Kind == "page" {
		n = min(n, 50)
		if !hex64.MatchString(v.Snapshot) || (c.ExpectedItems > 50) != (v.Cursor != "") {
			return false
		}
	}
	if len(v.Grades) != n {
		return false
	}
	seen := map[string]bool{}
	last := ""
	for _, g := range v.Grades {
		if !hex32.MatchString(g.ID) || seen[g.ID] || g.Version != c.ExpectedVersion || !hex32.MatchString(g.Release) || (last != "" && g.ID <= last) {
			return false
		}
		seen[g.ID] = true
		last = g.ID
	}
	return true
}

func Run(ctx context.Context, c Config) (Result, error) {
	if c.Validate() != nil {
		return Result{}, errConfig
	}
	// Self-signed TLS is accepted only after strict numeric loopback validation.
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, MaxConnsPerHost: c.Concurrency, MaxIdleConnsPerHost: c.Concurrency}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	r := Result{Kind: c.Kind, Items: c.ExpectedItems, Version: c.ExpectedVersion, Concurrency: c.Concurrency, Requests: c.Requests, Warmups: c.Warmups, ErrorCodes: map[string]int{}, Latencies: []float64{}, OS: runtime.GOOS, Arch: runtime.GOARCH, GoVersion: runtime.Version(), CPUs: runtime.NumCPU()}
	one := func() (float64, int, string) {
		path := c.BaseURL + "/v3/classes/" + c.ClassID + "/reports/grades/" + c.LearnerID
		method, body := "GET", ""
		if c.Kind == "page" {
			path += "/pages"
			method = "POST"
			body = `{"page_size":50}`
		}
		if c.Kind == "export" {
			path += "/exports"
			method = "POST"
			body = `{"format":"csv"}`
		}
		start := time.Now()
		req, e := http.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
		if e != nil {
			return 0, 0, "REQUEST"
		}
		req.AddCookie(&http.Cookie{Name: "__Host-lms-v3", Value: c.Session})
		req.Header.Set("Origin", c.BaseURL)
		req.Header.Set("X-CSRF-Token", c.CSRF)
		req.Header.Set("Content-Type", "application/json")
		resp, e := client.Do(req)
		if e != nil {
			return float64(time.Since(start).Nanoseconds()) / 1e6, 0, "TRANSPORT"
		}
		defer resp.Body.Close()
		b, e := io.ReadAll(io.LimitReader(resp.Body, 262145))
		elapsed := float64(time.Since(start).Nanoseconds()) / 1e6
		if e != nil || len(b) > 262144 {
			return elapsed, len(b), "BODY_BOUND"
		}
		if !contract(c, resp, b) {
			return elapsed, len(b), "CONTRACT"
		}
		return elapsed, len(b), ""
	}
	for i := 0; i < c.Warmups; i++ {
		_, _, code := one()
		if code != "" {
			r.WarmupErrors++
			r.ErrorCodes[code]++
		}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan struct{}, c.Requests)
	for i := 0; i < c.Requests; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	start := time.Now()
	for i := 0; i < c.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				ms, n, code := one()
				mu.Lock()
				r.Latencies = append(r.Latencies, ms)
				r.MaxBytes = max(r.MaxBytes, n)
				if code != "" {
					r.Errors++
					r.ErrorCodes[code]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	r.DurationMS = float64(time.Since(start).Nanoseconds()) / 1e6
	r.Throughput = float64(c.Requests) * 1000 / r.DurationMS
	r.P50 = Percentile(r.Latencies, .5)
	r.P95 = Percentile(r.Latencies, .95)
	r.P99 = Percentile(r.Latencies, .99)
	return r, nil
}
