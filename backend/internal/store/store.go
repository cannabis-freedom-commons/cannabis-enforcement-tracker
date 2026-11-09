package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/migrations"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var ErrConfiguration = errors.New("processing configuration is incomplete")
var ErrResearchAgent = errors.New("research agent exited before its first model step")

var ErrBudget = errors.New("daily research budget reached")

// Store is the operational database. DB is its one read-write connection (writers queue on it);
// Read, when set, is a small pool of read-only connections for the public pages' queries, which
// WAL lets run alongside a write instead of waiting behind every worker update.
type Store struct {
	DB   *sql.DB
	Read *sql.DB
	// the site's latest Pages build on GitHub, as the publication loop last saw it (worker/batch.go)
	deployRun atomic.Pointer[DeployRun]
}

// DeployRun is a run of the site's Pages workflow on GitHub.
type DeployRun struct {
	URL        string `json:"url"`
	Status     string `json:"status"`               // queued, in_progress, completed
	Conclusion string `json:"conclusion,omitempty"` // success, failure, cancelled… once completed
	HeadSHA    string `json:"head_sha"`
	StartedAt  int64  `json:"started_at"`
	UpdatedAt  int64  `json:"updated_at"`
	// IncludesBatch: the run builds the last publication batch (its commit contains the batch's)
	IncludesBatch bool `json:"includes_batch"`
}

func (s *Store) SetDeployRun(r *DeployRun) { s.deployRun.Store(r) }
func (s *Store) DeployRun() *DeployRun     { return s.deployRun.Load() }

func (s *Store) reader() *sql.DB {
	if s.Read != nil {
		return s.Read
	}
	return s.DB
}

// Reader is the connection pool for read-only queries.
func (s *Store) Reader() *sql.DB { return s.reader() }

// Close closes both connection pools.
func (s *Store) Close() error {
	if s.Read != nil {
		s.Read.Close()
	}
	return s.DB.Close()
}

func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(name TEXT PRIMARY KEY)`); err != nil {
		db.Close()
		return nil, err
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".sql" {
			continue
		}
		var n int
		if err = db.QueryRow("SELECT count(*) FROM schema_migrations WHERE name=?", e.Name()).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			continue
		}
		b, _ := migrations.Files.ReadFile(e.Name())
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(string(b)); err == nil {
			_, err = tx.Exec("INSERT INTO schema_migrations VALUES(?)", e.Name())
		}
		if err != nil {
			tx.Rollback()
			db.Close()
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
	}
	if path != ":memory:" {
		if err = os.Chmod(path, 0600); err != nil {
			db.Close()
			return nil, err
		}
	}
	st := &Store{DB: db}
	// a file path (not a DSN with its own options): read-only connections beside the writer
	if path != ":memory:" && !strings.ContainsAny(path, "?") && !strings.HasPrefix(path, "file:") {
		read, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=query_only(1)")
		if err == nil && read.Ping() == nil {
			read.SetMaxOpenConns(4)
			read.SetMaxIdleConns(4)
			st.Read = read
		} else if read != nil {
			read.Close()
		}
	}
	return st, nil
}

type History struct {
	Status      string `json:"status"`
	Explanation string `json:"explanation"`
	CreatedAt   int64  `json:"created_at"`
}
type Submission struct {
	ResumeAt     int64     `json:"resume_at,omitempty"`
	ID           string    `json:"id"`
	Origin       string    `json:"origin"`
	URL          string    `json:"url"`
	Note         string    `json:"note"`
	CaseID       string    `json:"case_id"`
	Status       string    `json:"status"`
	Explanation  string    `json:"explanation"`
	ReportSlug   string    `json:"report_slug,omitempty"`
	PRURL        string    `json:"pr_url,omitempty"`
	NostrEventID string    `json:"nostr_event_id,omitempty"`
	ReplyEventID string    `json:"reply_event_id,omitempty"`
	CreatedAt    int64     `json:"created_at"`
	History      []History `json:"history"`
	// public outputs: what the source check found, the AI's latest answer, the draft report built from it
	SourceCheck   json.RawMessage `json:"source_check,omitempty"`
	CommitSHA     string          `json:"commit_sha,omitempty"` // the report's commit in the report repository
	CommitURL     string          `json:"commit_url,omitempty"` // that commit on GitHub (set by the API)
	Validation    json.RawMessage `json:"validation,omitempty"` // how the AI's answer was checked against its sources
	Result        json.RawMessage `json:"result,omitempty"`
	DraftPath     string          `json:"draft_path,omitempty"`
	DraftMarkdown string          `json:"draft_markdown,omitempty"`
	Resumable     bool            `json:"resumable,omitempty"` // finished AI research is saved, so a retry can resume after it
}
type Intake struct {
	Origin, URL, CanonicalURL, Note, Author, EventID, EventRaw, Provenance, IdempotencyKey string
	Gateway                                                                                bool
}

func (s *Store) Accept(ctx context.Context, in Intake) (string, error) {
	normalized, err := safeurl.Normalize(in.CanonicalURL)
	if err != nil {
		return "", err
	}
	in.CanonicalURL = normalized
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if in.EventID != "" {
		var id string
		err = tx.QueryRowContext(ctx, "SELECT id FROM submissions WHERE origin_event_id=?", in.EventID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	if in.IdempotencyKey != "" {
		var id string
		err = tx.QueryRowContext(ctx, "SELECT submission_id FROM ingest_keys WHERE key=?", in.IdempotencyKey).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	now := time.Now().Unix()
	cid := ID()
	_, err = tx.ExecContext(ctx, `INSERT INTO cases(id,canonical_url,created_at,updated_at) VALUES(?,?,?,?) ON CONFLICT(canonical_url) DO NOTHING`, cid, in.CanonicalURL, now, now)
	if err != nil {
		return "", err
	}
	// The preceding case insert acquires SQLite's writer lock before checking
	// receipts, so concurrent intake cannot both observe a missing submission.
	// Reuse even terminal/failed receipts, before queue capacity or Nostr side effects.
	var existing string
	err = tx.QueryRowContext(ctx, "SELECT id FROM submissions WHERE canonical_url=? ORDER BY created_at,id LIMIT 1", in.CanonicalURL).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var status, explanation string
	if err = tx.QueryRowContext(ctx, "SELECT id,status,explanation FROM cases WHERE canonical_url=?", in.CanonicalURL).Scan(&cid, &status, &explanation); err != nil {
		return "", err
	}
	var depth int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE status IN ('queued','running')").Scan(&depth); err != nil {
		return "", err
	}
	if depth >= 1000 {
		return "", errors.New("queue full")
	}
	id := ID()
	var event any
	if in.EventID != "" {
		event = in.EventID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO submissions(id,origin,original_url,canonical_url,note,author,origin_event_id,case_id,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, in.Origin, in.URL, in.CanonicalURL, in.Note, in.Author, event, cid, now)
	if err != nil {
		return "", err
	}
	if in.IdempotencyKey != "" {
		if _, err = tx.ExecContext(ctx, "INSERT INTO ingest_keys VALUES(?,?)", in.IdempotencyKey, id); err != nil {
			return "", err
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO submission_history(submission_id,status,explanation,actor,created_at) VALUES(?,?,?,?,?)", id, status, explanation, "intake", now)
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO source_urls(url,case_id) VALUES(?,?) ON CONFLICT DO NOTHING", in.CanonicalURL, cid); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO jobs(id,kind,ref,payload,available_at) VALUES(?,?,?,?,?) ON CONFLICT DO NOTHING", ID(), "research", cid, in.Provenance, now); err != nil {
		return "", err
	}
	if in.EventID != "" {
		if _, err = tx.ExecContext(ctx, "INSERT INTO nostr_events VALUES(?,?,?,?,?) ON CONFLICT DO NOTHING", in.EventID, in.EventRaw, "in", "submission", now); err != nil {
			return "", err
		}
	}
	if in.Gateway {
		if err = Enqueue(tx, "outbox", "gateway", id, "{}", now); err != nil {
			return "", err
		}
	}
	if Terminal(status) && status != "committed" && (in.Origin == "nostr" || in.Gateway) {
		if err = Enqueue(tx, "outbox", "resolution", id+":"+status, "{}", now); err != nil {
			return "", err
		}
	}
	return id, tx.Commit()
}
func Enqueue(tx *sql.Tx, table, kind, ref, payload string, now int64) error {
	if table != "jobs" && table != "outbox" {
		return errors.New("invalid queue")
	}
	_, err := tx.Exec("INSERT INTO "+table+"(id,kind,ref,payload,available_at) VALUES(?,?,?,?,?) ON CONFLICT(kind,ref) DO NOTHING", ID(), kind, ref, payload, now)
	return err
}
func (s *Store) Get(ctx context.Context, id string) (Submission, error) {
	v := Submission{History: []History{}}
	var sourceCheck, result, validation string
	err := s.reader().QueryRowContext(ctx, `SELECT s.id,s.origin,s.original_url,s.note,s.case_id,c.status,c.explanation,c.report_slug,c.pr_url,coalesce(s.origin_event_id,nullif(s.gateway_event_id,''),''),s.reply_event_id,s.created_at,c.source_check,c.ai_result,c.validation,c.draft_path,c.draft_markdown,coalesce((SELECT commit_sha FROM local_publications WHERE case_id=c.id),''),coalesce((SELECT available_at FROM jobs WHERE ref=c.id AND kind='research' AND error_code='daily_budget_wait' AND status='queued'),0),EXISTS(SELECT 1 FROM research_checkpoints WHERE case_id=c.id) FROM submissions s JOIN cases c ON c.id=s.case_id WHERE s.id=?`, id).Scan(&v.ID, &v.Origin, &v.URL, &v.Note, &v.CaseID, &v.Status, &v.Explanation, &v.ReportSlug, &v.PRURL, &v.NostrEventID, &v.ReplyEventID, &v.CreatedAt, &sourceCheck, &result, &validation, &v.DraftPath, &v.DraftMarkdown, &v.CommitSHA, &v.ResumeAt, &v.Resumable)
	if sourceCheck != "" && json.Valid([]byte(sourceCheck)) {
		v.SourceCheck = json.RawMessage(sourceCheck)
	}
	if result != "" && json.Valid([]byte(result)) {
		v.Result = json.RawMessage(result)
	}
	if validation != "" && json.Valid([]byte(validation)) {
		v.Validation = json.RawMessage(validation)
	}
	if err != nil {
		return v, err
	}
	rows, err := s.reader().QueryContext(ctx, "SELECT status,explanation,created_at FROM submission_history WHERE submission_id=? ORDER BY id", id)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var h History
		if err = rows.Scan(&h.Status, &h.Explanation, &h.CreatedAt); err != nil {
			return v, err
		}
		v.History = append(v.History, h)
	}
	return v, rows.Err()
}

var states = map[string]bool{"budget_wait": true, "committed": true, "queued": true, "fetching": true, "classifying": true, "researching": true, "verifying": true, "drafted": true, "awaiting_review": true, "publishing": true, "merged": true, "deploying": true, "published": true, "duplicate": true, "not_relevant": true, "insufficient_evidence": true, "routed_to_cfi": true, "needs_review": true, "failed": true, "source_unavailable": true}

func Terminal(status string) bool {
	return status == "committed" || status == "published" || status == "duplicate" || status == "not_relevant" || status == "insufficient_evidence" || status == "routed_to_cfi" || status == "needs_review" || status == "failed" || status == "source_unavailable"
}

// SetCaseOutput records a public output of a case: "source_check", "ai_result", "validation", or the draft
// ("draft_path" with "draft_markdown").
func (s *Store) SetCaseOutput(ctx context.Context, cid string, values map[string]string) error {
	for column, value := range values {
		switch column {
		case "source_check", "ai_result", "validation", "draft_path", "draft_markdown":
		default:
			return errors.New("unknown case output")
		}
		if len(value) > 512*1024 {
			return errors.New("case output too large")
		}
		if _, err := s.DB.ExecContext(ctx, "UPDATE cases SET "+column+"=? WHERE id=?", value, cid); err != nil {
			return err
		}
	}
	return nil
}

// Transition and outgoing intent commit together. A local commit is distinct from live deployment.
func (s *Store) Transition(ctx context.Context, cid, status, explanation, slug, pr string) error {
	if !states[status] || len(explanation) > 2000 || explanation == "" {
		return errors.New("invalid transition")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	var prev string
	if err = tx.QueryRowContext(ctx, "SELECT status FROM cases WHERE id=?", cid).Scan(&prev); err != nil {
		return err
	}
	if prev == status {
		return nil
	}
	if status == "committed" {
		var recorded int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM local_publications WHERE case_id=?", cid).Scan(&recorded); err != nil {
			return err
		}
		if recorded != 1 || slug == "" {
			return errors.New("local commit prerequisites missing")
		}
	}
	if status == "published" {
		var report, proposal string
		var local int
		if err = tx.QueryRowContext(ctx, "SELECT report_slug,pr_url,(SELECT count(*) FROM local_publications WHERE case_id=?) FROM cases WHERE id=?", cid, cid).Scan(&report, &proposal, &local); err != nil {
			return err
		}
		if prev != "deploying" || report == "" || (proposal == "" && local == 0) {
			return errors.New("publication prerequisites missing")
		}
	}
	// A committed report goes on only to the site: on main (deploying), then live (published).
	if prev == "published" || (prev == "committed" && status != "deploying") {
		return errors.New("published case is immutable")
	}
	_, err = tx.ExecContext(ctx, "UPDATE cases SET status=?,explanation=?,report_slug=CASE WHEN ?='' THEN report_slug ELSE ? END,pr_url=CASE WHEN ?='' THEN pr_url ELSE ? END,updated_at=? WHERE id=?", status, explanation, slug, slug, pr, pr, now, cid)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO submission_history(submission_id,status,explanation,actor,created_at) SELECT id,?,?,?,? FROM submissions WHERE case_id=?`, status, explanation, "worker", now, cid)
	if err != nil {
		return err
	}
	// Nostr hears the outcome; a committed report only once it is on the site, with its link.
	if Terminal(status) && status != "committed" {
		rows, err := tx.Query("SELECT id FROM submissions WHERE case_id=? AND (origin='nostr' OR gateway_event_id<>'' OR id IN (SELECT ref FROM outbox WHERE kind='gateway'))", cid)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err = Enqueue(tx, "outbox", "resolution", id+":"+status, "{}", now); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

type Job struct {
	ID, Kind, Ref, Payload, Token string
	Attempts                      int
}

func (s *Store) Claim(ctx context.Context, table string, lease time.Duration) (Job, error) {
	return s.ClaimKind(ctx, table, "", lease)
}

// ClaimKind claims the next due job, of the given kind only when kind is set. Publish jobs come
// first: committing a validated draft takes seconds and should not wait behind research.
func (s *Store) ClaimKind(ctx context.Context, table, kind string, lease time.Duration) (Job, error) {
	if table != "jobs" && table != "outbox" {
		return Job{}, errors.New("invalid queue")
	}
	j := Job{Token: ID()}
	now := time.Now().Unix()
	err := s.DB.QueryRowContext(ctx, `UPDATE `+table+` SET status='running',lease_until=?,lease_token=?,attempts=attempts+1 WHERE id=(SELECT id FROM `+table+` WHERE ((status='queued' AND available_at<=?) OR (status='running' AND lease_until<?)) AND (?='' OR kind=?) ORDER BY kind='publish' DESC,available_at,id LIMIT 1) RETURNING id,kind,ref,payload,attempts`, now+int64(lease.Seconds()), j.Token, now, now, kind, kind).Scan(&j.ID, &j.Kind, &j.Ref, &j.Payload, &j.Attempts)
	return j, err
}
func (s *Store) Finish(ctx context.Context, table string, j Job, workErr error) error {
	if table != "jobs" && table != "outbox" {
		return errors.New("invalid queue")
	}
	status := "done"
	code := ""
	if workErr != nil {
		status = "queued"
		code = "delivery_or_processing_failed"
		if j.Attempts >= 8 {
			status = "dead"
		}
	}
	resumeAt := int64(0)
	delay := time.Duration(1<<min(j.Attempts, 10)) * time.Second
	refunded := 0
	if errors.Is(workErr, ErrBudget) {
		status = "queued"
		code = "daily_budget_wait"
		resumeAt = time.Now().UTC().Truncate(24 * time.Hour).Add(24*time.Hour + time.Minute).Unix()
		delay = time.Until(time.Unix(resumeAt, 0))
		refunded = 1
	}
	if errors.Is(workErr, ErrInterrupted) {
		status, code, delay, refunded = "queued", "server_interrupted", 0, 1
	}
	if errors.Is(workErr, ErrConfiguration) {
		status = "queued"
		code = "waiting_for_configuration"
		delay = 30 * time.Second
		refunded = 1
	}
	if errors.Is(workErr, ErrResearchAgent) {
		status, code, delay, refunded = "queued", "research_agent_unavailable", 2*time.Second, 1
	}
	res, err := s.DB.ExecContext(ctx, "UPDATE "+table+" SET status=?,error_code=?,available_at=?,lease_until=0,attempts=attempts-? WHERE id=? AND lease_token=? AND status='running'", status, code, time.Now().Add(delay).Unix(), refunded, j.ID, j.Token)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("lease lost")
	}
	if table == "jobs" && errors.Is(workErr, ErrBudget) {
		return s.Transition(ctx, j.Ref, "budget_wait", "The daily AI research reservation limit has been reached. No new AI run has started. Scheduled to resume at "+time.Unix(resumeAt, 0).UTC().Format(time.RFC3339)+".", "", "")
	}
	return nil
}
func (s *Store) Heartbeat(ctx context.Context, table string, j Job, lease time.Duration) error {
	if table != "jobs" && table != "outbox" {
		return errors.New("invalid queue")
	}
	r, err := s.DB.ExecContext(ctx, "UPDATE "+table+" SET lease_until=? WHERE id=? AND lease_token=? AND status='running'", time.Now().Add(lease).Unix(), j.ID, j.Token)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return errors.New("lease lost")
	}
	return nil
}
func (s *Store) Reserve(ctx context.Context, cid, workspace string, perRun, daily int) (string, error) {
	if perRun <= 0 || daily < 0 {
		return "", errors.New("invalid research budget")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var used int
	err = tx.QueryRowContext(ctx, "SELECT coalesce(sum(reserved_cents),0) FROM research_runs WHERE created_at>=?", time.Now().UTC().Truncate(24*time.Hour).Unix()).Scan(&used)
	if err != nil {
		return "", err
	}
	if daily > 0 && used+perRun > daily {
		return "", ErrBudget
	}
	id := ID()
	_, err = tx.ExecContext(ctx, "INSERT INTO research_runs(id,case_id,workspace,reserved_cents,created_at) VALUES(?,?,?,?,?)", id, cid, workspace, perRun, time.Now().Unix())
	if err != nil {
		return "", err
	}
	return id, tx.Commit()
}
func (s *Store) Backup(ctx context.Context, path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("backup destination must be new and writable")
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	if _, err = s.DB.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}
func JSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprint(err))
	}
	return string(b)
}
