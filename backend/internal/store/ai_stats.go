package store

import (
	"context"
	"sort"
)

// AIUsageBucket aggregates runs with the same pricing conditions and outcome. A representative
// start time preserves peak/off-peak estimation without exporting individual runs.
type AIUsageBucket struct {
	Model            string  `json:"model"`
	Category         string  `json:"category"`
	StartedAt        int64   `json:"started_at"`
	Runs             int64   `json:"runs"`
	Running          int64   `json:"running"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	// finished runs with both times, and the seconds they took in all
	Timed           int64 `json:"timed"`
	DurationSeconds int64 `json:"duration_seconds"`
}

// What a research run ended with, for comparing what each kind of work costs: a new report, an
// update of an existing one (another source for it), a submission outside the tracker's scope,
// another finished outcome (duplicate, too little evidence, review), or no outcome (failed,
// interrupted or still running).
const categorySQL = `CASE outcome
 WHEN 'new_event' THEN 'new'
 WHEN 'additional_source' THEN 'update' WHEN 'existing_case' THEN 'update'
 WHEN 'not_relevant' THEN 'out_of_scope' WHEN 'routed_to_cfi' THEN 'out_of_scope'
 WHEN '' THEN 'unfinished' ELSE 'other' END`

func (s *Store) AIUsage(ctx context.Context, since int64) ([]AIUsageBucket, error) {
	// Never join receipts: several submissions may share one research case/run.
	// Keep reported costs separate from token estimates, and split pricing tiers.
	rows, err := s.reader().QueryContext(ctx, `SELECT model,`+categorySQL+`,min(started_at),count(*),
 sum(CASE WHEN status='running' THEN 1 ELSE 0 END),
 sum(input_tokens),sum(output_tokens),sum(reasoning_tokens),
 sum(cache_read_tokens),sum(cache_write_tokens),sum(cost_usd),
 sum(CASE WHEN finished_at>=started_at THEN 1 ELSE 0 END),
 coalesce(sum(CASE WHEN finished_at>=started_at THEN finished_at-started_at END),0)
 FROM research_runs WHERE started_at>0 AND started_at>=?
 GROUP BY model,2,cost_usd>0,
 (CAST(strftime('%w',started_at,'unixepoch') AS INTEGER) BETWEEN 1 AND 5
 AND (CAST(strftime('%H',started_at,'unixepoch') AS INTEGER) BETWEEN 1 AND 3
 OR CAST(strftime('%H',started_at,'unixepoch') AS INTEGER) BETWEEN 6 AND 9))
 ORDER BY model,2,min(started_at)`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AIUsageBucket{}
	for rows.Next() {
		var v AIUsageBucket
		if err := rows.Scan(&v.Model, &v.Category, &v.StartedAt, &v.Runs, &v.Running, &v.InputTokens, &v.OutputTokens, &v.ReasoningTokens, &v.CacheReadTokens, &v.CacheWriteTokens, &v.CostUSD, &v.Timed, &v.DurationSeconds); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// AIDuration is how long finished runs of one model and outcome took: the median and the slowest
// tenth's threshold (p90), in seconds. Only these summaries leave the service, never single runs.
type AIDuration struct {
	Model    string `json:"model"`
	Category string `json:"category"`
	Runs     int64  `json:"runs"`
	Median   int64  `json:"median_seconds"`
	P90      int64  `json:"p90_seconds"`
}

func (s *Store) AIDurations(ctx context.Context, since int64) ([]AIDuration, error) {
	rows, err := s.reader().QueryContext(ctx, `SELECT model,`+categorySQL+`,finished_at-started_at
 FROM research_runs WHERE started_at>0 AND started_at>=? AND finished_at>=started_at
 ORDER BY 1,2,3`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type key struct{ model, category string }
	groups := map[key][]int64{}
	order := []key{}
	for rows.Next() {
		var k key
		var d int64
		if err := rows.Scan(&k.model, &k.category, &d); err != nil {
			return nil, err
		}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]AIDuration, 0, len(order))
	for _, k := range order {
		d := groups[k]
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		out = append(out, AIDuration{Model: k.model, Category: k.category, Runs: int64(len(d)), Median: percentile(d, 50), P90: percentile(d, 90)})
	}
	return out, nil
}

// percentile of sorted values, nearest rank
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	i := (p*len(sorted)+99)/100 - 1
	if i < 0 {
		i = 0
	}
	return sorted[i]
}
