package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestCapitalPunishmentRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.DB.Close() }()
	ids := []string{}
	for i, reason := range []string{
		"Death sentences belong to the CFI history.",
		"Executions belong to the CFI history.",
		"A change in the law belongs to the CFI history.",
		"An execution was already committed.",
		"Executions belong to CFI, but this job is still active.",
	} {
		url := "https://news.example/" + ID()
		id, err := s.Accept(ctx, Intake{Origin: "web", URL: url, CanonicalURL: url})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		v, _ := s.Get(ctx, id)
		job, err := s.Claim(ctx, "jobs", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		status := "routed_to_cfi"
		if err := s.Transition(ctx, v.CaseID, status, reason, "", ""); err != nil {
			t.Fatal(err)
		}
		if i == 3 {
			// A successful case must remain untouched, regardless of its explanation.
			if _, err := s.DB.Exec("UPDATE cases SET status='committed' WHERE id=?", v.CaseID); err != nil {
				t.Fatal(err)
			}
		}
		if i != 4 {
			if err := s.Finish(ctx, "jobs", job, nil); err != nil {
				t.Fatal(err)
			}
		}
		s.DB.Exec("INSERT INTO research_checkpoints(case_id,payload,created_at) VALUES(?,?,?)", v.CaseID, "{}", 1)
		s.SetCaseOutput(ctx, v.CaseID, map[string]string{"ai_result": `{"outcome":"routed_to_cfi"}`})
	}
	if _, err := s.DB.Exec("DELETE FROM schema_migrations WHERE name='017_capital_punishment_recovery.sql'"); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		v, err := s.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		want := "routed_to_cfi"
		if i < 2 {
			want = "queued"
		}
		if i == 3 {
			want = "committed"
		}
		if v.Status != want {
			t.Fatalf("%d: got %s, want %s", i, v.Status, want)
		}
		var saved int
		s.DB.QueryRow("SELECT count(*) FROM research_checkpoints WHERE case_id=?", v.CaseID).Scan(&saved)
		if i < 2 {
			if saved != 0 || len(v.Result) != 0 || len(v.History) != 3 {
				t.Fatalf("stale decision or missing history: %+v", v)
			}
		} else if saved != 1 {
			t.Fatal("unrelated checkpoint removed")
		}
	}
	next, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil || next.Kind != "research" || next.Attempts != 1 {
		t.Fatal(next, err)
	}
}
