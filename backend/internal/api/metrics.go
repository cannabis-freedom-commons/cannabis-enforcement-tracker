package api

// GET /metrics: the backend's statistics in the Prometheus text format, for Prometheus (or Grafana
// Alloy) to scrape and Grafana to chart. Bearer METRICS_TOKEN (or INGEST_TOKEN). Database
// figures are read at each scrape; HTTP figures are counted since the backend started. Remote
// workers report theirs to the backend, which lists them here too (worker.API.WriteMetrics).

import (
	"bufio"
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var started = time.Now()

// Prom writes metrics in the Prometheus text format; each metric's HELP and TYPE come once.
type Prom struct {
	w    io.Writer
	seen map[string]bool
}

func NewProm(w io.Writer) *Prom { return &Prom{w: w, seen: map[string]bool{}} }

// Metric declares a metric (type gauge, counter or summary).
func (p *Prom) Metric(name, kind, help string) {
	if p.seen[name] {
		return
	}
	p.seen[name] = true
	fmt.Fprintf(p.w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
}

// Sample writes one value; labels are name, value pairs.
func (p *Prom) Sample(name string, value float64, labels ...string) {
	var b strings.Builder
	b.WriteString(name)
	if len(labels) > 0 {
		b.WriteByte('{')
		for i := 0; i+1 < len(labels); i += 2 {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(labels[i])
			b.WriteString(`="`)
			b.WriteString(strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(labels[i+1]))
			b.WriteByte('"')
		}
		b.WriteByte('}')
	}
	fmt.Fprintf(p.w, "%s %s\n", b.String(), strconv.FormatFloat(value, 'f', -1, 64))
}

func (s *Server) metricsToken() string {
	if s.Config.MetricsToken != "" {
		return s.Config.MetricsToken
	}
	return s.Config.IngestToken
}

func (s *Server) prometheus(w http.ResponseWriter, r *http.Request) {
	want := s.metricsToken()
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if want == "" {
		problem(w, 404, "Metrics are not configured.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
		problem(w, 401, "Unauthorized.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	out := bufio.NewWriter(w)
	defer out.Flush()
	p := NewProm(out)
	if err := s.databaseMetrics(ctx, p); err != nil {
		p.Metric("tracker_metrics_error", "gauge", "1 when the database figures could not be read at this scrape.")
		p.Sample("tracker_metrics_error", 1)
	}
	if s.Health != nil {
		ready := 0.0
		if s.Health.Ready() {
			ready = 1
		}
		p.Metric("tracker_research_ready", "gauge", "1 when research can take jobs (a worker or local agent is ready).")
		p.Sample("tracker_research_ready", ready)
	}
	if s.WriteWorkerMetrics != nil {
		s.WriteWorkerMetrics(p)
	}
	writeHTTPMetrics(p)
	p.Metric("tracker_start_time_seconds", "gauge", "When the backend started (Unix time).")
	p.Sample("tracker_start_time_seconds", float64(started.Unix()))
	if info, err := os.Stat(s.Config.Database); err == nil {
		p.Metric("tracker_database_bytes", "gauge", "Size of the SQLite database file.")
		p.Sample("tracker_database_bytes", float64(info.Size()))
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	p.Metric("go_goroutines", "gauge", "Goroutines of the backend.")
	p.Sample("go_goroutines", float64(runtime.NumGoroutine()))
	p.Metric("go_memstats_heap_alloc_bytes", "gauge", "Heap memory in use.")
	p.Sample("go_memstats_heap_alloc_bytes", float64(m.HeapAlloc))
	p.Metric("go_memstats_sys_bytes", "gauge", "Memory obtained from the system.")
	p.Sample("go_memstats_sys_bytes", float64(m.Sys))
}

// databaseMetrics: the queue, submissions, research runs and their cost, publications, budget.
func (s *Server) databaseMetrics(ctx context.Context, p *Prom) error {
	db := s.Store.Reader()
	type row = []any
	query := func(q string, scan func(row) error, dest ...any) error {
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := rows.Scan(dest...); err != nil {
				return err
			}
			if err := scan(dest); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	var a, b string
	var n, x, y, z, v, cost float64

	p.Metric("tracker_jobs", "gauge", "Jobs in the queue by kind (research, publish) and status.")
	if err := query("SELECT kind,status,count(*) FROM jobs GROUP BY 1,2", func(row) error {
		p.Sample("tracker_jobs", n, "kind", a, "status", b)
		return nil
	}, &a, &b, &n); err != nil {
		return err
	}
	p.Metric("tracker_outbox", "gauge", "Nostr messages waiting to be sent, by status.")
	if err := query("SELECT status,count(*) FROM outbox GROUP BY 1", func(row) error {
		p.Sample("tracker_outbox", n, "status", a)
		return nil
	}, &a, &n); err != nil {
		return err
	}
	now := time.Now().Unix()
	var oldest *int64
	if err := db.QueryRowContext(ctx, "SELECT min(available_at) FROM jobs WHERE kind='research' AND status='queued' AND available_at<=?", now).Scan(&oldest); err != nil {
		return err
	}
	wait := 0.0
	if oldest != nil {
		wait = float64(now - *oldest)
	}
	p.Metric("tracker_research_queue_oldest_seconds", "gauge", "How long the longest-waiting due research job has waited (0 when none).")
	p.Sample("tracker_research_queue_oldest_seconds", wait)

	p.Metric("tracker_cases", "gauge", "Submitted sources (cases) by status.")
	if err := query("SELECT status,count(*) FROM cases GROUP BY 1", func(row) error {
		p.Sample("tracker_cases", n, "status", a)
		return nil
	}, &a, &n); err != nil {
		return err
	}
	p.Metric("tracker_submissions_total", "counter", "Submissions received, by origin (web, nostr, rss for news discovery, ingest).")
	if err := query("SELECT origin,count(*) FROM submissions GROUP BY 1", func(row) error {
		p.Sample("tracker_submissions_total", n, "origin", a)
		return nil
	}, &a, &n); err != nil {
		return err
	}

	p.Metric("tracker_research_runs_total", "counter", "AI research runs by model and status.")
	if err := query("SELECT model,status,count(*) FROM research_runs GROUP BY 1,2", func(row) error {
		p.Sample("tracker_research_runs_total", n, "model", a, "status", b)
		return nil
	}, &a, &b, &n); err != nil {
		return err
	}
	p.Metric("tracker_research_outcomes_total", "counter", "Completed research runs by the outcome they proposed.")
	if err := query("SELECT outcome,count(*) FROM research_runs WHERE status='completed' GROUP BY 1", func(row) error {
		p.Sample("tracker_research_outcomes_total", n, "outcome", a)
		return nil
	}, &a, &n); err != nil {
		return err
	}
	p.Metric("tracker_research_tokens_total", "counter", "Model tokens used by research, by model and type.")
	p.Metric("tracker_research_cost_usd_total", "counter", "Research cost at API prices, by model.")
	p.Metric("tracker_research_run_seconds", "summary", "Duration of finished research runs, by model.")
	var cacheWrite, seconds, runs float64
	if err := query(`SELECT model,sum(input_tokens),sum(output_tokens),sum(reasoning_tokens),sum(cache_read_tokens),sum(cache_write_tokens),sum(cost_usd),
		sum(CASE WHEN finished_at>started_at AND started_at>0 THEN finished_at-started_at ELSE 0 END),
		sum(CASE WHEN finished_at>started_at AND started_at>0 THEN 1 ELSE 0 END)
		FROM research_runs GROUP BY 1`, func(row) error {
		p.Sample("tracker_research_tokens_total", x, "model", a, "type", "input")
		p.Sample("tracker_research_tokens_total", y, "model", a, "type", "output")
		p.Sample("tracker_research_tokens_total", z, "model", a, "type", "reasoning")
		p.Sample("tracker_research_tokens_total", v, "model", a, "type", "cache_read")
		p.Sample("tracker_research_tokens_total", cacheWrite, "model", a, "type", "cache_write")
		p.Sample("tracker_research_cost_usd_total", cost, "model", a)
		p.Sample("tracker_research_run_seconds_sum", seconds, "model", a)
		p.Sample("tracker_research_run_seconds_count", runs, "model", a)
		return nil
	}, &a, &x, &y, &z, &v, &cacheWrite, &cost, &seconds, &runs); err != nil {
		return err
	}

	p.Metric("tracker_reports_committed_total", "counter", "Reports committed by the backend (new reports and sources added).")
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM local_publications").Scan(&n); err != nil {
		return err
	}
	p.Sample("tracker_reports_committed_total", n)

	var reserved float64
	day := time.Now().UTC().Truncate(24 * time.Hour).Unix()
	if err := db.QueryRowContext(ctx, "SELECT coalesce(sum(reserved_cents),0) FROM research_runs WHERE created_at>=?", day).Scan(&reserved); err != nil {
		return err
	}
	p.Metric("tracker_budget_reserved_cents", "gauge", "Research budget reserved today (UTC).")
	p.Sample("tracker_budget_reserved_cents", reserved)
	p.Metric("tracker_budget_daily_limit_cents", "gauge", "Daily research budget (0 = no limit).")
	p.Sample("tracker_budget_daily_limit_cents", float64(s.Config.DailyCents))

	p.Metric("tracker_harvest_seen_items", "gauge", "News items discovery has seen, by provider.")
	if err := query("SELECT substr(provider,1,instr(provider||':',':')-1),count(*) FROM harvest_state WHERE provider LIKE '%:item:%' GROUP BY 1", func(row) error {
		p.Sample("tracker_harvest_seen_items", n, "provider", a)
		return nil
	}, &a, &n); err != nil {
		return err
	}
	return nil
}

// HTTP request counts and durations by route, since the backend started.
type httpStat struct {
	count   map[string]float64 // route + "\x00" + status class
	seconds map[string]float64 // route
	n       map[string]float64 // route
}

var httpStats = struct {
	sync.Mutex
	httpStat
}{httpStat: httpStat{count: map[string]float64{}, seconds: map[string]float64{}, n: map[string]float64{}}}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(b)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.status == 0 {
		w.status = 101
	}
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

// Instrument counts requests by route (the matched pattern) and status class. Long-lived streams
// (WebSockets, event streams) count once, when they end.
func Instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		route := r.Pattern
		if route == "" || route == "/" {
			route = "unmatched"
		}
		status := sw.status
		if status == 0 {
			status = 200
		}
		class := strconv.Itoa(status/100) + "xx"
		elapsed := time.Since(start).Seconds()
		httpStats.Lock()
		httpStats.count[route+"\x00"+class]++
		if status != 101 && !strings.HasSuffix(route, "/stream") {
			httpStats.seconds[route] += elapsed
			httpStats.n[route]++
		}
		httpStats.Unlock()
	})
}

func writeHTTPMetrics(p *Prom) {
	httpStats.Lock()
	defer httpStats.Unlock()
	p.Metric("tracker_http_requests_total", "counter", "HTTP requests by route and status class, since the backend started.")
	keys := make([]string, 0, len(httpStats.count))
	for k := range httpStats.count {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		route, class, _ := strings.Cut(k, "\x00")
		p.Sample("tracker_http_requests_total", httpStats.count[k], "route", route, "code", class)
	}
	p.Metric("tracker_http_request_seconds", "summary", "Time to answer HTTP requests by route (streams left out).")
	routes := make([]string, 0, len(httpStats.n))
	for r := range httpStats.n {
		routes = append(routes, r)
	}
	sort.Strings(routes)
	for _, r := range routes {
		p.Sample("tracker_http_request_seconds_sum", math.Round(httpStats.seconds[r]*1e6)/1e6, "route", r)
		p.Sample("tracker_http_request_seconds_count", httpStats.n[r], "route", r)
	}
}
