package research

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/archivefile"
	"path/filepath"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// BackfillTokens counts again, from their saved event logs, the tokens of runs recorded before
// reasoning and cached input were kept apart: until then reasoning tokens (billed as output) went
// uncounted, which made a run look about 40% cheaper than it was. A run without its log keeps its
// counts. Each run is counted once.
func BackfillTokens(ctx context.Context, s *store.Store) error {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,workspace FROM research_runs WHERE tokens_backfilled=0 AND status NOT IN ('running','starting','preparing')")
	if err != nil {
		return err
	}
	type run struct{ id, dir string }
	var runs []run
	for rows.Next() {
		var r run
		if err = rows.Scan(&r.id, &r.dir); err != nil {
			rows.Close()
			return err
		}
		runs = append(runs, r)
	}
	rows.Close()
	for _, r := range runs {
		t, ok := countTokens(filepath.Join(r.dir, "research-events.jsonl"))
		query, args := "UPDATE research_runs SET tokens_backfilled=1 WHERE id=?", []any{r.id}
		if ok {
			query = "UPDATE research_runs SET input_tokens=?,output_tokens=?,reasoning_tokens=?,cache_read_tokens=?,cache_write_tokens=?,tokens_backfilled=1 WHERE id=?"
			args = []any{t[0], t[1], t[2], t[3], t[4], r.id}
		}
		if _, err = s.DB.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

// input, output, reasoning, cache read, cache write over every finished step of a run's log
func countTokens(path string) ([5]int64, bool) {
	var total [5]int64
	f, err := archivefile.Open(path)
	if err != nil || path == "research-events.jsonl" {
		return total, false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	steps := 0
	for scanner.Scan() {
		var e executionEvent
		if json.Unmarshal(scanner.Bytes(), &e) != nil || e.Type != "step_finish" {
			continue
		}
		t := e.Part.Tokens
		total[0] += max(0, t.Input)
		total[1] += max(0, t.Output)
		total[2] += max(0, t.Reasoning)
		total[3] += max(0, t.Cache.Read)
		total[4] += max(0, t.Cache.Write)
		steps++
	}
	return total, steps > 0 && scanner.Err() == nil
}
