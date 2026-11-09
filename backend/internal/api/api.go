package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"io"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

type bucket struct {
	N     int
	Until time.Time
}
type Limiter struct {
	mu      sync.Mutex
	entries map[string]bucket
}

func (l *Limiter) Allow(key string, limit int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.entries == nil {
		l.entries = map[string]bucket{}
	}
	now := time.Now()
	for k, b := range l.entries {
		if now.After(b.Until) {
			delete(l.entries, k)
		}
	}
	b, exists := l.entries[key]
	if !exists && len(l.entries) >= 10000 {
		return false
	}
	if b.Until.IsZero() {
		b.Until = now.Add(time.Hour)
	}
	b.N++
	l.entries[key] = b
	return b.N <= limit
}

type Server struct {
	Store  *store.Store
	Health *research.Health
	// RemoteAgents lists the research agents of connected workers (RESEARCH_MODE=remote).
	RemoteAgents func() ([]config.ResearchAgent, string)
	// WriteWorkerMetrics adds the remote workers' figures to /metrics.
	WriteWorkerMetrics func(*Prom)
	Config             config.Config
	Limit              Limiter
	streamMu           sync.Mutex
	streamCount        int
	activityMu         sync.Mutex
	activityCache      map[string]cachedActivity
}

var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, code int, msg string) {
	write(w, code, map[string]string{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if s.Store.DB.PingContext(r.Context()) != nil {
			problem(w, 503, "unavailable")
			return
		}
		write(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/submissions", s.submit)
	mux.HandleFunc("GET /v1/submissions/{id}", s.get)
	mux.HandleFunc("POST /v1/submissions/{id}/restart", s.restart)
	mux.HandleFunc("GET /v1/submissions/{id}/source/screenshot", s.sourceScreenshot)
	mux.HandleFunc("GET /v1/submissions/{id}/ai", s.ai)
	mux.HandleFunc("GET /v1/submissions/{id}/ai/log", s.aiLog)
	mux.HandleFunc("GET /v1/submissions/{id}/ai/raw", s.aiRaw)
	mux.HandleFunc("GET /v1/submissions", s.submissions)
	mux.HandleFunc("GET /v1/submissions/stream", s.submissionStream)
	mux.HandleFunc("GET /v1/submissions/activity", s.submissionActivity)
	mux.HandleFunc("GET /v1/activity", s.activity)
	mux.HandleFunc("GET /v1/status", s.status)
	mux.HandleFunc("GET /v1/stats/ai", s.aiStats)
	mux.HandleFunc("GET /internal/metrics", s.metrics)
	mux.HandleFunc("GET /metrics", s.prometheus)
	mux.HandleFunc("POST /v1/ingest/candidate", s.ingest)
	mux.HandleFunc("POST /v1/webhooks/github", s.webhook)
	mux.HandleFunc("GET /v1/config", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"nostr_enabled": s.Config.BotSecret != "", "relays": s.Config.Relays, "processing": s.processingInfo(r.Context())})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Add("Vary", "Origin")
			allowed := false
			for _, v := range s.Config.Origins {
				if origin == v {
					allowed = true
				}
			}
			if !allowed {
				problem(w, 403, "origin not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	host := s.clientIP(r)
	local := s.localDevelopmentRequest(r)
	if !local && !s.Limit.Allow("ip:"+host, 20) {
		w.Header().Set("Retry-After", "3600")
		problem(w, 429, "Too many submissions. Try again in an hour.")
		return
	}
	var in struct {
		URL  string `json:"url"`
		Note string `json:"note"`
	}
	if decode(w, r, &in) != nil {
		problem(w, 400, "Invalid submission JSON.")
		return
	}
	normalized, err := safeurl.Normalize(in.URL)
	if err != nil {
		problem(w, 422, "Use a public HTTP(S) news URL on its standard port.")
		return
	}
	if !utf8.ValidString(in.Note) || utf8.RuneCountInString(in.Note) > 500 || strings.IndexFunc(in.Note, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) >= 0 {
		problem(w, 422, "The public note must be at most 500 characters.")
		return
	}
	uhost := strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(normalized, "https://"), "http://"), "/", 2)[0]
	if !local && !s.Limit.Allow("host:"+uhost, 100) {
		w.Header().Set("Retry-After", "3600")
		problem(w, 429, "This news host has reached its intake limit.")
		return
	}
	id, err := s.Store.Accept(r.Context(), store.Intake{Origin: "web", URL: in.URL, CanonicalURL: normalized, Note: strings.TrimSpace(in.Note), Gateway: s.Config.BotSecret != ""})
	if err != nil {
		problem(w, 503, "Intake is unavailable. Please try again later.")
		return
	}
	s.accepted(w, r, id)
}
func (s *Server) accepted(w http.ResponseWriter, r *http.Request, id string) {
	v, err := s.Store.Get(r.Context(), id)
	if err != nil {
		problem(w, 503, "Status temporarily unavailable.")
		return
	}
	write(w, 202, map[string]any{"id": id, "status": v.Status, "status_url": s.Config.PublicURL + "/submission/?id=" + id, "nostr_event_id": nil})
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		problem(w, 404, "Receipt not found.")
		return
	}
	v, err := s.Store.Get(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		problem(w, 404, "Receipt not found.")
		return
	}
	if err != nil {
		problem(w, 503, "Status unavailable.")
		return
	}
	// the report's commit on GitHub, linked whether it has been pushed yet or not
	if regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(v.CommitSHA) {
		v.CommitURL = "https://github.com/" + s.Config.GitHubRepo + "/commit/" + v.CommitSHA
	}
	write(w, 200, v)
}
func (s *Server) batch(w http.ResponseWriter, r *http.Request) {
	ids := strings.Split(r.URL.Query().Get("ids"), ",")
	if len(ids) > 50 {
		problem(w, 400, "At most 50 receipts per request.")
		return
	}
	out := []store.Submission{}
	for _, id := range ids {
		if !idPattern.MatchString(id) {
			problem(w, 400, "Invalid receipt ID.")
			return
		}
		v, err := s.Store.Get(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			problem(w, 503, "Status unavailable.")
			return
		}
		out = append(out, v)
	}
	write(w, 200, map[string]any{"submissions": out})
}
func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if s.Config.IngestToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.Config.IngestToken)) != 1 {
		problem(w, 401, "Unauthorized.")
		return
	}
	var in struct {
		URL        string `json:"url"`
		Origin     string `json:"origin"`
		Provenance string `json:"provenance"`
		Key        string `json:"idempotency_key"`
	}
	if decode(w, r, &in) != nil || len(in.Provenance) > 2000 || len(in.Key) < 1 || len(in.Key) > 256 || (in.Origin != "rss" && in.Origin != "gdelt" && in.Origin != "api") {
		problem(w, 400, "URL, origin, provenance and idempotency_key required.")
		return
	}
	u, err := safeurl.Normalize(in.URL)
	if err != nil {
		problem(w, 422, "Invalid URL.")
		return
	}
	id, err := s.Store.Accept(r.Context(), store.Intake{Origin: in.Origin, URL: in.URL, CanonicalURL: u, Provenance: in.Provenance, IdempotencyKey: in.Origin + ":" + in.Key})
	if err != nil {
		problem(w, 503, "Intake unavailable.")
		return
	}
	s.accepted(w, r, id)
}
func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	if s.Config.WebhookSecret == "" {
		problem(w, 503, "Webhook not configured.")
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024*1024))
	if err != nil {
		problem(w, 400, "Invalid webhook.")
		return
	}
	mac := hmac.New(sha256.New, []byte(s.Config.WebhookSecret))
	mac.Write(b)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-Hub-Signature-256"))) {
		problem(w, 401, "Invalid signature.")
		return
	}
	// Webhooks are wakeup hints only; authoritative reconciliation polls GitHub by PR SHA.
	var payload map[string]any
	if json.Unmarshal(b, &payload) != nil {
		problem(w, 400, "Invalid JSON.")
		return
	}
	write(w, 202, map[string]string{"status": "accepted for periodic reconciliation"})
}

// Activity is keyset-paginated by immutable history ID and excludes machine discovery.
func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	cursor := int64(1 << 62)
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			problem(w, 400, "Invalid activity cursor.")
			return
		}
		cursor = parsed
	}
	rows, err := s.Store.Reader().QueryContext(r.Context(), `SELECT h.id,h.submission_id,h.status,h.explanation,h.created_at FROM submission_history h JOIN submissions s ON s.id=h.submission_id WHERE h.id<? AND s.origin IN ('web','nostr') AND h.status IN ('published','duplicate','not_relevant','insufficient_evidence','routed_to_cfi','needs_review','failed','source_unavailable') ORDER BY h.id DESC LIMIT 26`, cursor)
	if err != nil {
		problem(w, 503, "Activity unavailable.")
		return
	}
	defer rows.Close()
	type event struct {
		Cursor       int64  `json:"cursor"`
		SubmissionID string `json:"submission_id"`
		Status       string `json:"status"`
		Explanation  string `json:"explanation"`
		CreatedAt    int64  `json:"created_at"`
	}
	out := []event{}
	for rows.Next() {
		var e event
		if err = rows.Scan(&e.Cursor, &e.SubmissionID, &e.Status, &e.Explanation, &e.CreatedAt); err != nil {
			problem(w, 503, "Activity unavailable.")
			return
		}
		out = append(out, e)
	}
	if rows.Err() != nil {
		problem(w, 503, "Activity unavailable.")
		return
	}
	next := ""
	if len(out) > 25 {
		out = out[:25]
		next = strconv.FormatInt(out[24].Cursor, 10)
	}
	write(w, 200, map[string]any{"events": out, "next_cursor": next})
}
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if s.Config.IngestToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.Config.IngestToken)) != 1 {
		problem(w, 401, "Unauthorized.")
		return
	}
	counts := map[string]int{}
	for _, table := range []string{"jobs", "outbox"} {
		for _, status := range []string{"queued", "running", "dead"} {
			var n int
			if err := s.Store.Reader().QueryRowContext(r.Context(), "SELECT count(*) FROM "+table+" WHERE status=?", status).Scan(&n); err != nil {
				problem(w, 503, "Metrics unavailable.")
				return
			}
			counts[table+"_"+status] = n
		}
	}
	write(w, 200, counts)
}

// Forwarded addresses are trusted only from explicitly configured proxy networks.
func (s *Server) clientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	trusted := func(ip netip.Addr) bool {
		for _, raw := range s.Config.TrustedProxies {
			prefix, err := netip.ParsePrefix(raw)
			if err == nil && prefix.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !trusted(ip) {
		return ip.String()
	}
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(forwarded) > 10 {
		return ip.String()
	}
	for i := len(forwarded) - 1; i >= 0; i-- {
		candidate, err := netip.ParseAddr(strings.TrimSpace(forwarded[i]))
		if err != nil {
			return ip.String()
		}
		ip = candidate
		if !trusted(candidate) {
			break
		}
	}
	return ip.String()
}
