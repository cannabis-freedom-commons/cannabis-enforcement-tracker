package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

type AIEvent struct {
	ID        int64  `json:"id"`
	RunID     string `json:"run_id"`
	Role      string `json:"role"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	CreatedAt int64  `json:"created_at"`
}
type AIRun struct {
	ID           string `json:"id"`
	Model        string `json:"model"`
	Runner       string `json:"runner"`
	SessionID    string `json:"session_id,omitempty"`
	Status       string `json:"status"`
	Outcome      string `json:"outcome,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	StartedAt    int64  `json:"started_at"`
	FinishedAt   int64  `json:"finished_at"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	// reasoning is billed as output; cached input has its own price
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}
type AIActivity struct {
	Runs       []AIRun   `json:"runs"`
	Events     []AIEvent `json:"events"`
	NextCursor string    `json:"next_cursor"`
}

func (s *Store) StartRun(ctx context.Context, id, model, runner string) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE research_runs SET model=?,runner=?,status='running',started_at=? WHERE id=?", model, runner, time.Now().Unix(), id)
	return err
}
func (s *Store) EndRun(ctx context.Context, id, status, outcome string) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE research_runs SET status=?,outcome=?,finished_at=? WHERE id=?", status, outcome, time.Now().Unix(), id)
	return err
}

// Only deliberate public activity belongs here; raw prompts, articles, arguments,
// reasoning, provider error bodies and credentials stay outside the public transcript.
func (s *Store) AddAIEvent(ctx context.Context, cid, run, role, kind, text string) error {
	if role != "user" && role != "assistant" && role != "tool" && role != "system" {
		return errors.New("invalid activity role")
	}
	// a final answer with its quotations (often in Thai or another script of three bytes a character) is long
	if len(text) > 64*1024 || strings.TrimSpace(text) == "" {
		return errors.New("invalid activity text")
	}
	_, err := s.DB.ExecContext(ctx, "INSERT INTO ai_events(case_id,run_id,role,kind,text,created_at) VALUES(?,?,?,?,?,?)", cid, run, role, kind, text, time.Now().Unix())
	return err
}
func (s *Store) AIActivity(ctx context.Context, submissionID string, after int64) (AIActivity, error) {
	return s.aiActivity(ctx, submissionID, after, false)
}

// AIActivityLog returns every recorded public event and run for an explicit JSON export.
func (s *Store) AIActivityLog(ctx context.Context, submissionID string) (AIActivity, error) {
	return s.aiActivity(ctx, submissionID, 0, true)
}
func (s *Store) aiActivity(ctx context.Context, submissionID string, after int64, full bool) (AIActivity, error) {
	out := AIActivity{Runs: []AIRun{}, Events: []AIEvent{}}
	var cid string
	if err := s.reader().QueryRowContext(ctx, "SELECT case_id FROM submissions WHERE id=?", submissionID).Scan(&cid); err != nil {
		return out, err
	}
	runLimit, eventLimit := 10, 101
	if full {
		runLimit, eventLimit = -1, -1
	}
	rows, err := s.reader().QueryContext(ctx, "SELECT id,model,runner,session_id,status,outcome,created_at,coalesce(started_at,0),coalesce(finished_at,0),input_tokens,output_tokens,reasoning_tokens,cache_read_tokens,cache_write_tokens,cost_usd FROM research_runs WHERE case_id=? ORDER BY rowid DESC LIMIT ?", cid, runLimit)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r AIRun
		if err = rows.Scan(&r.ID, &r.Model, &r.Runner, &r.SessionID, &r.Status, &r.Outcome, &r.CreatedAt, &r.StartedAt, &r.FinishedAt, &r.InputTokens, &r.OutputTokens, &r.ReasoningTokens, &r.CacheReadTokens, &r.CacheWriteTokens, &r.CostUSD); err != nil {
			rows.Close()
			return out, err
		}
		out.Runs = append(out.Runs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	order := "ASC"
	floor := after
	if after < 0 {
		order = "DESC"
		floor = 0
	}
	rows, err = s.reader().QueryContext(ctx, "SELECT id,run_id,role,kind,text,created_at FROM ai_events WHERE case_id=? AND id>? ORDER BY id "+order+" LIMIT ?", cid, floor, eventLimit)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e AIEvent
		if err = rows.Scan(&e.ID, &e.RunID, &e.Role, &e.Kind, &e.Text, &e.CreatedAt); err != nil {
			return out, err
		}
		out.Events = append(out.Events, e)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if !full && len(out.Events) > 100 {
		out.Events = out.Events[:100]
		if after >= 0 {
			out.NextCursor = strconv.FormatInt(out.Events[99].ID, 10)
		}
	}
	if after < 0 {
		for i, j := 0, len(out.Events)-1; i < j; i, j = i+1, j-1 {
			out.Events[i], out.Events[j] = out.Events[j], out.Events[i]
		}
	}
	return out, nil
}
func IsMissingAI(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// RunUsage is one model step's token use and cost, added to its run.
type RunUsage struct {
	Input, Output, Reasoning, CacheRead, CacheWrite int64
	Cost                                            float64
}

// RunUpdate changes a research run's details; nil fields stay as they are.
type RunUpdate struct {
	Model, Runner, Status, Workspace, ContextMode, SessionID *string
	Usage                                                    *RunUsage
}

// UpdateRun records what a research run learned about itself: its workspace, starting-context
// mode, OpenCode session and the tokens each step used.
func (s *Store) UpdateRun(ctx context.Context, id string, u RunUpdate) error {
	set := func(column string, v *string) error {
		if v == nil {
			return nil
		}
		_, err := s.DB.ExecContext(ctx, "UPDATE research_runs SET "+column+"=? WHERE id=?", *v, id)
		return err
	}
	for _, f := range []struct {
		column string
		v      *string
	}{{"model", u.Model}, {"runner", u.Runner}, {"status", u.Status}, {"workspace", u.Workspace}, {"context_mode", u.ContextMode}} {
		if err := set(f.column, f.v); err != nil {
			return err
		}
	}
	if u.SessionID != nil {
		if _, err := s.DB.ExecContext(ctx, "UPDATE research_runs SET session_id=? WHERE id=? AND session_id<>?", *u.SessionID, id, *u.SessionID); err != nil {
			return err
		}
	}
	if t := u.Usage; t != nil {
		if _, err := s.DB.ExecContext(ctx, "UPDATE research_runs SET input_tokens=input_tokens+?,output_tokens=output_tokens+?,reasoning_tokens=reasoning_tokens+?,cache_read_tokens=cache_read_tokens+?,cache_write_tokens=cache_write_tokens+?,cost_usd=cost_usd+?,tokens_backfilled=1 WHERE id=?", t.Input, t.Output, max(0, t.Reasoning), max(0, t.CacheRead), max(0, t.CacheWrite), t.Cost, id); err != nil {
			return err
		}
	}
	return nil
}

// FeedChanged tells open request pages to refresh (the agents' health changed, for example).
func (s *Store) FeedChanged(ctx context.Context) {
	_, _ = s.DB.ExecContext(ctx, "UPDATE submission_feed_version SET version=version+1 WHERE singleton=1")
}
