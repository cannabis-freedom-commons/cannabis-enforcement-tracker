package api

import (
	"context"
	"time"
)

// These are the reservations used by the worker's guard, not provider invoices.
type BudgetInfo struct {
	Unlimited       bool  `json:"unlimited"`
	ReservedCents   int64 `json:"reserved_cents"`
	DailyLimitCents int64 `json:"daily_limit_cents"`
	PerRunCents     int64 `json:"per_run_cents"`
	RemainingCents  int64 `json:"remaining_cents"`
	RunsRemaining   int64 `json:"runs_remaining"`
	ResetsAt        int64 `json:"resets_at"`
}

func (s *Server) budgetInfo(ctx context.Context, now time.Time) *BudgetInfo {
	if s.Store == nil {
		return nil
	}
	start := now.UTC().Truncate(24 * time.Hour)
	b := &BudgetInfo{DailyLimitCents: int64(s.Config.DailyCents), PerRunCents: int64(s.Config.RunCents), ResetsAt: start.Add(24 * time.Hour).Unix()}
	if err := s.Store.Reader().QueryRowContext(ctx, "SELECT coalesce(sum(reserved_cents),0) FROM research_runs WHERE created_at>=?", start.Unix()).Scan(&b.ReservedCents); err != nil {
		return nil
	}
	if b.DailyLimitCents == 0 {
		b.Unlimited = true
		b.RemainingCents, b.RunsRemaining = -1, -1
		return b
	}
	b.RemainingCents = max(int64(0), b.DailyLimitCents-b.ReservedCents)
	if b.PerRunCents > 0 {
		b.RunsRemaining = b.RemainingCents / b.PerRunCents
	}
	return b
}
