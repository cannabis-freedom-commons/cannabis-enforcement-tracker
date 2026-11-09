package store

import (
	"context"
	"sort"
	"time"
)

// Outcomes are the submissions page's statistics: what became of every submitted source, where
// sources come from, how many arrived and were published lately, and how long the way from a
// source to a committed report takes.
type Outcomes struct {
	ByStatus  map[string]int64 `json:"by_status"` // cases
	ByOrigin  map[string]int64 `json:"by_origin"` // submissions
	Received  Window           `json:"received"`  // submissions
	Committed Window           `json:"committed"` // reports committed
	Published Window           `json:"published"` // cases that reached the site
	// minutes from a case's first submission to its committed report, last 7 days
	MinutesToCommit *Spread `json:"minutes_to_commit,omitempty"`
	UpdatedAt       int64   `json:"updated_at"`
}
type Window struct {
	Day  int64 `json:"day"`
	Week int64 `json:"week"`
}
type Spread struct {
	Count  int   `json:"count"`
	Median int64 `json:"median"`
	P90    int64 `json:"p90"`
}

func (s *Store) Outcomes(ctx context.Context) (Outcomes, error) {
	db := s.Reader()
	now := time.Now().Unix()
	day, week := now-86400, now-7*86400
	out := Outcomes{ByStatus: map[string]int64{}, ByOrigin: map[string]int64{}, UpdatedAt: now}
	group := func(q string, into map[string]int64) error {
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int64
			if err := rows.Scan(&k, &n); err != nil {
				return err
			}
			into[k] = n
		}
		return rows.Err()
	}
	if err := group("SELECT status,count(*) FROM cases GROUP BY 1", out.ByStatus); err != nil {
		return out, err
	}
	if err := group("SELECT origin,count(*) FROM submissions GROUP BY 1", out.ByOrigin); err != nil {
		return out, err
	}
	if err := db.QueryRowContext(ctx, "SELECT count(CASE WHEN created_at>? THEN 1 END),count(*) FROM submissions WHERE created_at>?", day, week).Scan(&out.Received.Day, &out.Received.Week); err != nil {
		return out, err
	}
	if err := db.QueryRowContext(ctx, "SELECT count(CASE WHEN created_at>? THEN 1 END),count(*) FROM local_publications WHERE created_at>?", day, week).Scan(&out.Committed.Day, &out.Committed.Week); err != nil {
		return out, err
	}
	if err := db.QueryRowContext(ctx, `SELECT count(CASE WHEN h.created_at>? THEN 1 END),count(*) FROM (
 SELECT s.case_id, max(h.created_at) created_at FROM submission_history h JOIN submissions s ON s.id=h.submission_id
 WHERE h.status='published' AND h.actor<>'migration' AND h.created_at>? GROUP BY s.case_id) h`, day, week).Scan(&out.Published.Day, &out.Published.Week); err != nil {
		return out, err
	}
	rows, err := db.QueryContext(ctx, `SELECT p.created_at-(SELECT min(created_at) FROM submissions WHERE case_id=p.case_id) FROM local_publications p WHERE p.created_at>?`, week)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	var minutes []int64
	for rows.Next() {
		var seconds *int64
		if err := rows.Scan(&seconds); err != nil {
			return out, err
		}
		if seconds != nil && *seconds >= 0 {
			minutes = append(minutes, *seconds/60)
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if len(minutes) > 0 {
		sort.Slice(minutes, func(i, j int) bool { return minutes[i] < minutes[j] })
		out.MinutesToCommit = &Spread{Count: len(minutes), Median: minutes[len(minutes)/2], P90: minutes[min(len(minutes)-1, len(minutes)*9/10)]}
	}
	return out, nil
}
