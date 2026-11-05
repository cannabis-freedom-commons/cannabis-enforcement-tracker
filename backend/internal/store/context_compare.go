package store

import (
	"context"
	"sort"
)

// ContextStats compares research by starting-context mode (RESEARCH_CONTEXT). A case belongs to
// the mode of its first run that recorded one; "first run" figures are that run alone.
type ContextStats struct {
	Mode                           string
	Cases, Runs                    int
	FirstSteps, FirstSeconds       float64 // medians
	FirstContextTokens, FirstOut   float64 // means: input + cache read, and output + reasoning
	CostPerCase                    float64 // mean USD over all runs of the case
	Corrected, Committed, Rejected int     // cases: needed a correction; committed; failed
	Statuses                       map[string]int
}

func (s *Store) CompareContext(ctx context.Context) ([]ContextStats, error) {
	rows, err := s.DB.QueryContext(ctx, `WITH first AS (
  SELECT r.case_id, r.context_mode,
    ROW_NUMBER() OVER (PARTITION BY r.case_id ORDER BY r.created_at, r.id) AS n, r.id
  FROM research_runs r WHERE r.context_mode<>'')
SELECT f.context_mode, c.id, c.status,
  (SELECT count(*) FROM research_runs WHERE case_id=c.id),
  (SELECT coalesce(sum(cost_usd),0) FROM research_runs WHERE case_id=c.id),
  (SELECT count(*) FROM ai_events WHERE run_id=f.id AND kind='step'),
  coalesce(r.finished_at-r.started_at,0), r.input_tokens+r.cache_read_tokens, r.output_tokens+r.reasoning_tokens, r.status,
  EXISTS(SELECT 1 FROM ai_events WHERE case_id=c.id AND role='user' AND kind='request' AND text LIKE 'The answer did not pass%')
   OR EXISTS(SELECT 1 FROM submission_history h JOIN submissions s ON s.id=h.submission_id WHERE s.case_id=c.id AND h.explanation LIKE '%needs a correction%' AND h.created_at>=r.created_at)
FROM first f JOIN cases c ON c.id=f.case_id JOIN research_runs r ON r.id=f.id WHERE f.n=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type acc struct {
		ContextStats
		steps, seconds []float64
		ctxTokens, out float64
		finished       int
	}
	by := map[string]*acc{}
	for rows.Next() {
		var mode, id, status, firstStatus string
		var runs, steps int
		var cost float64
		var seconds, ctxTokens, out int64
		var corrected bool
		if err = rows.Scan(&mode, &id, &status, &runs, &cost, &steps, &seconds, &ctxTokens, &out, &firstStatus, &corrected); err != nil {
			return nil, err
		}
		a := by[mode]
		if a == nil {
			a = &acc{ContextStats: ContextStats{Mode: mode, Statuses: map[string]int{}}}
			by[mode] = a
		}
		a.Cases++
		a.Runs += runs
		a.CostPerCase += cost
		a.Statuses[status]++
		if status == "committed" || status == "published" {
			a.Committed++
		}
		if status == "failed" {
			a.Rejected++
		}
		if corrected {
			a.Corrected++
		}
		if firstStatus == "completed" {
			a.finished++
			a.steps = append(a.steps, float64(steps))
			a.seconds = append(a.seconds, float64(seconds))
			a.ctxTokens += float64(ctxTokens)
			a.out += float64(out)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var out []ContextStats
	for _, a := range by {
		if a.Cases > 0 {
			a.CostPerCase /= float64(a.Cases)
		}
		if a.finished > 0 {
			a.FirstContextTokens, a.FirstOut = a.ctxTokens/float64(a.finished), a.out/float64(a.finished)
		}
		a.FirstSteps, a.FirstSeconds = median(a.steps), median(a.seconds)
		out = append(out, a.ContextStats)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mode < out[j].Mode })
	return out, nil
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	if len(v)%2 == 1 {
		return v[len(v)/2]
	}
	return (v[len(v)/2-1] + v[len(v)/2]) / 2
}
