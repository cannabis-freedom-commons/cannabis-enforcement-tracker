package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCompareContextGroupsCasesByFirstRunMode(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ctx := context.Background()
	add := func(url, status, mode string, runs int, steps int, corrected bool) {
		id, err := s.Accept(ctx, Intake{Origin: "web", URL: url, CanonicalURL: url})
		if err != nil {
			t.Fatal(err)
		}
		v, _ := s.Get(ctx, id)
		if _, err = s.DB.Exec("UPDATE cases SET status=? WHERE id=?", status, v.CaseID); err != nil {
			t.Fatal(err)
		}
		for i := range runs {
			run, err := s.Reserve(ctx, v.CaseID, "", 1, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.DB.Exec("UPDATE research_runs SET context_mode=?,status='completed',started_at=100,finished_at=160,input_tokens=1000,cache_read_tokens=500,output_tokens=50,cost_usd=0.02,created_at=? WHERE id=?", mode, 1000+i, run); err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				for range steps {
					if err = s.AddAIEvent(ctx, v.CaseID, run, "system", "step", "Step done"); err != nil {
						t.Fatal(err)
					}
				}
			}
			if i == 1 && corrected {
				if err = s.AddAIEvent(ctx, v.CaseID, run, "user", "request", "The answer did not pass the service checks. Correct it."); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	add("https://a.example/1", "committed", "inline", 1, 8, false)
	add("https://a.example/2", "failed", "inline", 2, 12, true)
	add("https://a.example/3", "committed", "files", 1, 14, false)
	add("https://a.example/4", "duplicate", "", 1, 3, false) // before the switch: left out
	stats, err := s.CompareContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 || stats[0].Mode != "files" || stats[1].Mode != "inline" {
		t.Fatal(stats)
	}
	in := stats[1]
	if in.Cases != 2 || in.Runs != 3 || in.FirstSteps != 10 || in.FirstSeconds != 60 || in.FirstContextTokens != 1500 || in.Corrected != 1 || in.Committed != 1 || in.Rejected != 1 {
		t.Fatalf("%+v", in)
	}
	if in.CostPerCase < 0.0299 || in.CostPerCase > 0.0301 {
		t.Fatal(in.CostPerCase)
	}
}
