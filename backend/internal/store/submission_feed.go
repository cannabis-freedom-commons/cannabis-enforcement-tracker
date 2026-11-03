package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const SubmissionPageSize = 50

type SubmissionEntry struct {
	LastStage    string `json:"last_stage,omitempty"`
	ResumeAt     int64  `json:"resume_at,omitempty"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	RunCount     int64  `json:"run_count"`
	// the request's AI runs, newest first: model, times and tokens, for its duration and cost
	Runs         json.RawMessage `json:"runs"`
	ID           string          `json:"id"`
	Origin       string          `json:"origin"`
	URL          string          `json:"url"`
	Note         string          `json:"note"`
	CaseID       string          `json:"case_id"`
	Status       string          `json:"status"`
	Explanation  string          `json:"explanation"`
	ReportSlug   string          `json:"report_slug,omitempty"`
	PRURL        string          `json:"pr_url,omitempty"`
	NostrEventID string          `json:"nostr_event_id,omitempty"`
	ReplyEventID string          `json:"reply_event_id,omitempty"`
	CreatedAt    int64           `json:"created_at"`
	UpdatedAt    int64           `json:"updated_at"`
	sequence     int64
	sortKey      string
}
type SubmissionSummary struct {
	Total             int64 `json:"total"`
	Running           int64 `json:"running"`
	Queued            int64 `json:"queued"`
	Paused            int64 `json:"paused"`
	CommittedLastHour int64 `json:"committed_last_hour"`
}

type SubmissionPage struct {
	Working     []SubmissionEntry `json:"working"`
	Summary     SubmissionSummary `json:"summary"`
	Submissions []SubmissionEntry `json:"submissions"`
	Total       int64             `json:"total"`
	NextCursor  string            `json:"next_cursor"`
	Revision    string            `json:"revision"`
}

func ValidSubmissionFilter(status, origin string) bool {
	return (status == "" || states[status]) && (origin == "" || origin == "web" || origin == "nostr" || origin == "rss" || origin == "gdelt" || origin == "api")
}
func (s *Store) FeedVersion(ctx context.Context) (string, error) {
	var v int64
	err := s.DB.QueryRowContext(ctx, "SELECT version FROM submission_feed_version WHERE singleton=1").Scan(&v)
	return strconv.FormatInt(v, 10), err
}

// A weekday hour inside the model's peak window, matching the prices shown in the register.
const submissionPeakHours = `(cast(strftime('%w', coalesce(started_at,0), 'unixepoch') as integer) BETWEEN 1 AND 5 AND (cast(strftime('%H', coalesce(started_at,0), 'unixepoch') as integer) BETWEEN 1 AND 3 OR cast(strftime('%H', coalesce(started_at,0), 'unixepoch') as integer) BETWEEN 6 AND 9))`

// submissionSorts are the register columns a reader can order by. Each expression is fixed SQL,
// never taken from the request. Numeric sorts use integers so a page boundary stays exact.
var submissionSorts = map[string]struct {
	expr string
	text bool
}{
	"received": {expr: "s.created_at"},
	"updated":  {expr: "c.updated_at"},
	"id":       {expr: "s.id", text: true},
	"source":   {expr: "lower(s.original_url)", text: true},
	"status":   {expr: "c.status", text: true},
	"tokens":   {expr: "coalesce((SELECT sum(input_tokens+cache_read_tokens+output_tokens+reasoning_tokens) FROM research_runs WHERE case_id=c.id),0)"},
	"runs":     {expr: "(CASE WHEN c.updated_at>s.created_at THEN c.updated_at ELSE s.created_at END)-s.created_at"},
	"cost":     {expr: `coalesce((SELECT sum(CASE WHEN ifnull(cost_usd,0)>0 THEN cast(round(cost_usd*1000000000) as integer) WHEN model='opencode-go/muse-spark-1.3-contributor' OR instr(model,'opencode-go/muse-spark-1.3-contributor#')=1 THEN (ifnull(input_tokens,0)+ifnull(cache_write_tokens,0))*100+(ifnull(output_tokens,0)+ifnull(reasoning_tokens,0))*200+ifnull(cache_read_tokens,0)*2 WHEN model='swancloud/muse-spark-1.3-contributor' OR instr(model,'swancloud/muse-spark-1.3-contributor#')=1 THEN (ifnull(input_tokens,0)+ifnull(cache_write_tokens,0)+ifnull(cache_read_tokens,0))*100+(ifnull(output_tokens,0)+ifnull(reasoning_tokens,0))*200 WHEN model='swancloud/deepseek-v4.1-flash' OR instr(model,'swancloud/deepseek-v4.1-flash#')=1 THEN (ifnull(input_tokens,0)+ifnull(cache_write_tokens,0)+ifnull(cache_read_tokens,0))*150+(ifnull(output_tokens,0)+ifnull(reasoning_tokens,0))*90 WHEN model='opencode-go/deepseek-v4.1-flash' OR instr(model,'opencode-go/deepseek-v4.1-flash#')=1 THEN (ifnull(input_tokens,0)+ifnull(cache_write_tokens,0))*(CASE WHEN ` + submissionPeakHours + ` THEN 300 ELSE 150 END)+(ifnull(output_tokens,0)+ifnull(reasoning_tokens,0))*(CASE WHEN ` + submissionPeakHours + ` THEN 1200 ELSE 600 END)+ifnull(cache_read_tokens,0)*(CASE WHEN ` + submissionPeakHours + ` THEN 6 ELSE 3 END) ELSE 0 END) FROM research_runs WHERE case_id=c.id),0)`},
}

func normalizeSubmissionSort(sortBy, order string) (string, string, error) {
	if sortBy == "" {
		sortBy = "updated"
	}
	if order == "" {
		order = "desc"
	}
	if _, ok := submissionSorts[sortBy]; !ok || (order != "asc" && order != "desc") {
		return "", "", errors.New("invalid submission sort")
	}
	return sortBy, order, nil
}

func ValidSubmissionSort(sortBy, order string) bool {
	_, _, err := normalizeSubmissionSort(sortBy, order)
	return err == nil
}

type submissionCursor struct {
	sort, order, key string
	rowID            int64
}

// FormatSubmissionCursor records the sort and the last row so the next page cannot skip a tie.
func FormatSubmissionCursor(sortBy, order, key string, rowID int64) string {
	payload := sortBy + "\x1e" + order + "\x1e" + key + "\x1e" + strconv.FormatInt(rowID, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// ParseSubmissionCursor reads a page boundary. An empty cursor is the first page.
func ParseSubmissionCursor(raw string) (submissionCursor, error) {
	if raw == "" {
		return submissionCursor{}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return submissionCursor{}, errors.New("invalid submission cursor")
	}
	parts := strings.Split(string(b), "\x1e")
	if len(parts) != 4 {
		return submissionCursor{}, errors.New("invalid submission cursor")
	}
	rowID, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || rowID < 1 {
		return submissionCursor{}, errors.New("invalid submission cursor")
	}
	if _, _, err = normalizeSubmissionSort(parts[0], parts[1]); err != nil {
		return submissionCursor{}, errors.New("invalid submission cursor")
	}
	return submissionCursor{sort: parts[0], order: parts[1], key: parts[2], rowID: rowID}, nil
}

// ListSubmissions returns a bounded public projection of every intake origin.
// Pages follow the requested column, then receipt sequence, so equal values are not skipped.
func (s *Store) ListSubmissions(ctx context.Context, cursor, status, origin, sortBy, order string) (SubmissionPage, error) {
	out := SubmissionPage{Submissions: []SubmissionEntry{}, Working: []SubmissionEntry{}}
	sortBy, order, err := normalizeSubmissionSort(sortBy, order)
	if err != nil || !ValidSubmissionFilter(status, origin) {
		return out, errors.New("invalid submission filters")
	}
	page, err := ParseSubmissionCursor(cursor)
	if err != nil || (cursor != "" && (page.sort != sortBy || page.order != order)) {
		return out, errors.New("invalid submission filters")
	}
	spec := submissionSorts[sortBy]
	dir, cmp := "ASC", ">"
	if order == "desc" {
		dir, cmp = "DESC", "<"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT
      (SELECT count(*) FROM submissions),
      count(CASE WHEN EXISTS(SELECT 1 FROM jobs j WHERE j.ref=c.id AND j.kind='research' AND j.status='running' AND j.lease_until>unixepoch()) THEN 1 END),
      count(CASE WHEN c.status<>'budget_wait' AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.ref=c.id AND j.status='running' AND j.lease_until>unixepoch()) THEN 1 END),
      count(CASE WHEN c.status='budget_wait' THEN 1 END)
      FROM cases c
      WHERE c.status IN ('queued','budget_wait','fetching','classifying','researching','verifying','drafted','awaiting_review','publishing','merged','deploying')`).Scan(&out.Summary.Total, &out.Summary.Running, &out.Summary.Queued, &out.Summary.Paused); err != nil {
		return out, err
	}
	var revision int64
	// One publication per case, independent of receipt count, current status and list filters.
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM local_publications WHERE created_at>unixepoch()-3600 AND created_at<=unixepoch()`).Scan(&out.Summary.CommittedLastHour); err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT version FROM submission_feed_version WHERE singleton=1").Scan(&revision); err != nil {
		return out, err
	}
	out.Revision = strconv.FormatInt(revision, 10)
	where := " WHERE 1=1"
	args := []any{}
	if status != "" {
		where += " AND c.status=?"
		args = append(args, status)
	}
	if origin != "" {
		where += " AND s.origin=?"
		args = append(args, origin)
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM submissions s JOIN cases c ON c.id=s.case_id"+where, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if cursor != "" {
		var bound any = page.key
		if !spec.text {
			n, conv := strconv.ParseInt(page.key, 10, 64)
			if conv != nil {
				return out, errors.New("invalid submission filters")
			}
			bound = n
		}
		where += fmt.Sprintf(" AND (%s%s? OR (%s=? AND s.rowid%s?))", spec.expr, cmp, spec.expr, cmp)
		args = append(args, bound, bound, page.rowID)
	}
	columns := `SELECT s.rowid,s.id,s.origin,s.original_url,s.note,s.case_id,c.status,c.explanation,c.report_slug,c.pr_url,coalesce(s.origin_event_id,nullif(s.gateway_event_id,''),''),s.reply_event_id,s.created_at,c.updated_at,
 coalesce((SELECT status FROM submission_history WHERE submission_id=s.id AND status IN ('queued','fetching','classifying','researching','verifying','drafted','publishing','committed','published') ORDER BY id DESC LIMIT 1),''),
 coalesce((SELECT sum(input_tokens+cache_read_tokens) FROM research_runs WHERE case_id=c.id),0),
 coalesce((SELECT sum(output_tokens+reasoning_tokens) FROM research_runs WHERE case_id=c.id),0),
 (SELECT count(*) FROM research_runs WHERE case_id=c.id),
 coalesce((SELECT available_at FROM jobs WHERE ref=c.id AND kind='research' AND error_code='daily_budget_wait' AND status='queued'),0),
 coalesce((SELECT json_group_array(json_object('model',model,'status',status,'started_at',coalesce(started_at,0),'finished_at',coalesce(finished_at,0),'input_tokens',input_tokens,'output_tokens',output_tokens,'reasoning_tokens',reasoning_tokens,'cache_read_tokens',cache_read_tokens,'cache_write_tokens',cache_write_tokens,'cost_usd',cost_usd)) FROM (SELECT * FROM research_runs WHERE case_id=c.id ORDER BY rowid DESC LIMIT 20)),'[]')`
	from := ` FROM submissions s JOIN cases c ON c.id=s.case_id`
	// Select the bounded page before loading histories, usage totals and run JSON.
	// Otherwise SQLite may hydrate every candidate while sorting the full register.
	pageQuery := fmt.Sprintf("WITH page AS MATERIALIZED (SELECT s.rowid AS sequence, %s AS sort_key", spec.expr) + from + where + fmt.Sprintf(" ORDER BY %s %s, s.rowid %s LIMIT 51) ", spec.expr, dir, dir)
	rows, err := tx.QueryContext(ctx, pageQuery+columns+", CAST(page.sort_key AS TEXT) FROM page JOIN submissions s ON s.rowid=page.sequence JOIN cases c ON c.id=s.case_id"+fmt.Sprintf(" ORDER BY page.sort_key %s, s.rowid %s", dir, dir), args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v SubmissionEntry
		var runs, sortKey string
		if err = rows.Scan(&v.sequence, &v.ID, &v.Origin, &v.URL, &v.Note, &v.CaseID, &v.Status, &v.Explanation, &v.ReportSlug, &v.PRURL, &v.NostrEventID, &v.ReplyEventID, &v.CreatedAt, &v.UpdatedAt, &v.LastStage, &v.InputTokens, &v.OutputTokens, &v.RunCount, &v.ResumeAt, &runs, &sortKey); err != nil {
			rows.Close()
			return out, err
		}
		v.Runs = json.RawMessage(runs)
		v.sortKey = sortKey
		out.Submissions = append(out.Submissions, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Submissions) > SubmissionPageSize {
		out.Submissions = out.Submissions[:SubmissionPageSize]
		last := out.Submissions[SubmissionPageSize-1]
		out.NextCursor = FormatSubmissionCursor(sortBy, order, last.sortKey, last.sequence)
	}
	// working: the requests a job is running for now, and drafts waiting seconds for their commit.
	// A request left at an in-progress status with no job running (after a restart, or waiting to
	// retry) is queued, not working: Running counts research jobs actually running, never more than
	// the worker allows.
	active, err := tx.QueryContext(ctx, columns+from+` WHERE
      (EXISTS(SELECT 1 FROM jobs j WHERE j.ref=c.id AND j.status='running' AND j.lease_until>unixepoch())
       OR (c.status IN ('drafted','publishing') AND EXISTS(SELECT 1 FROM jobs j WHERE j.ref=c.id AND j.kind='publish' AND j.status='queued')))
      AND s.rowid=(SELECT max(latest.rowid) FROM submissions latest WHERE latest.case_id=c.id)
      ORDER BY c.updated_at DESC,s.rowid DESC LIMIT 50`)
	if err != nil {
		return out, err
	}
	for active.Next() {
		var v SubmissionEntry
		var runs string
		if err = active.Scan(&v.sequence, &v.ID, &v.Origin, &v.URL, &v.Note, &v.CaseID, &v.Status, &v.Explanation, &v.ReportSlug, &v.PRURL, &v.NostrEventID, &v.ReplyEventID, &v.CreatedAt, &v.UpdatedAt, &v.LastStage, &v.InputTokens, &v.OutputTokens, &v.RunCount, &v.ResumeAt, &runs); err != nil {
			active.Close()
			return out, err
		}
		v.Runs = json.RawMessage(runs)
		out.Working = append(out.Working, v)
	}
	err = active.Err()
	active.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
