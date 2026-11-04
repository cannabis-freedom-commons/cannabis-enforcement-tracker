package store

import (
	"context"
	"testing"
	"time"
)

func TestAIUsageCountsRunsOnceAndSeparatesPricing(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := s.Accept(ctx, Intake{Origin: "web", URL: "https://news.example/usage", CanonicalURL: "https://news.example/usage"}); err != nil {
			t.Fatal(err)
		}
	}
	var cid string
	if err := s.DB.QueryRow("SELECT id FROM cases LIMIT 1").Scan(&cid); err != nil {
		t.Fatal(err)
	}
	// Thursday off-peak, peak, reported cost, Friday off-peak, Saturday's
	// same peak hour (off-peak on weekends), then an unstarted reservation.
	times := []string{"2026-10-29T12:00:00Z", "2026-10-29T07:00:00Z", "2026-10-29T12:30:00Z", "2026-10-30T12:00:00Z", "2026-10-31T07:00:00Z", ""}
	for i, date := range times {
		id, err := s.Reserve(ctx, cid, "private-workspace", 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		if date == "" {
			continue
		}
		stamp, _ := time.Parse(time.RFC3339, date)
		cost := 0.0
		if i == 2 {
			cost = 0.4
		}
		if _, err = s.DB.Exec(`UPDATE research_runs SET model='test-model',started_at=?,status='failed',input_tokens=100,output_tokens=20,reasoning_tokens=5,cache_read_tokens=50,cache_write_tokens=2,cost_usd=? WHERE id=?`, stamp.Unix(), cost, id); err != nil {
			t.Fatal(err)
		}
	}
	buckets, err := s.AIUsage(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 3 {
		t.Fatalf("expected separate peak, off-peak, reported-cost buckets: %+v", buckets)
	}
	var runs, input, cache, output, reasoning int64
	var cost float64
	for _, b := range buckets {
		runs += b.Runs
		input += b.InputTokens
		cache += b.CacheReadTokens
		output += b.OutputTokens
		reasoning += b.ReasoningTokens
		cost += b.CostUSD
	}
	if runs != 5 || input != 500 || cache != 250 || output != 100 || reasoning != 25 || cost != 0.4 {
		t.Fatalf("incorrect totals: %+v", buckets)
	}
	since, _ := time.Parse(time.RFC3339, "2026-10-30T00:00:00Z")
	recent, err := s.AIUsage(ctx, since.Unix())
	if err != nil || len(recent) != 1 || recent[0].Runs != 2 {
		t.Fatal(recent, err)
	}
}

func TestAIUsageSplitsOutcomesAndTimesRuns(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if _, err := s.Accept(ctx, Intake{Origin: "web", URL: "https://news.example/outcomes", CanonicalURL: "https://news.example/outcomes"}); err != nil {
		t.Fatal(err)
	}
	var cid string
	if err := s.DB.QueryRow("SELECT id FROM cases LIMIT 1").Scan(&cid); err != nil {
		t.Fatal(err)
	}
	start, _ := time.Parse(time.RFC3339, "2026-10-29T12:00:00Z")
	// outcome, seconds taken (0: still running)
	runs := []struct {
		outcome string
		took    int64
	}{{"new_event", 100}, {"new_event", 300}, {"new_event", 200}, {"additional_source", 50}, {"existing_case", 70}, {"not_relevant", 10}, {"routed_to_cfi", 20}, {"duplicate", 30}, {"", 0}}
	for _, r := range runs {
		id, err := s.Reserve(ctx, cid, "private-workspace", 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		var finished any
		status := "running"
		if r.took > 0 {
			finished, status = start.Unix()+r.took, "completed"
		}
		if _, err = s.DB.Exec(`UPDATE research_runs SET model='m',outcome=?,status=?,started_at=?,finished_at=? WHERE id=?`, r.outcome, status, start.Unix(), finished, id); err != nil {
			t.Fatal(err)
		}
	}
	buckets, err := s.AIUsage(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][3]int64{}
	for _, b := range buckets {
		got[b.Category] = [3]int64{b.Runs, b.Timed, b.DurationSeconds}
	}
	want := map[string][3]int64{"new": {3, 3, 600}, "update": {2, 2, 120}, "out_of_scope": {2, 2, 30}, "other": {1, 1, 30}, "unfinished": {1, 0, 0}}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %v, want %v (all %v)", k, got[k], v, got)
		}
	}
	durations, err := s.AIDurations(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range durations {
		if d.Category == "new" && (d.Runs != 3 || d.Median != 200 || d.P90 != 300) {
			t.Fatalf("new-report durations: %+v", d)
		}
		if d.Category == "unfinished" {
			t.Fatalf("a running run has no duration: %+v", d)
		}
	}
}
