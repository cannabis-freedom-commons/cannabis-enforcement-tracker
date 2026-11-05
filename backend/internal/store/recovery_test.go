package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestRecoveryAfterReopenAcrossStages(t *testing.T) {
	for _, stage := range []string{"queued", "fetching", "classifying", "researching", "drafted", "publishing", "merged", "deploying"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "recovery.sqlite")
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			id := accept(t, s, false)
			v, _ := s.Get(ctx, id)
			old, err := s.Claim(ctx, "jobs", time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			s.Transition(ctx, v.CaseID, stage, "In progress before interruption.", "", "")
			run, _ := s.Reserve(ctx, v.CaseID, "private", 1, 100)
			s.StartRun(ctx, run, "actual/model", "opencode")
			s.DB.Exec("INSERT INTO research_checkpoints VALUES(?,?,?)", v.CaseID, `{"saved":"output"}`, time.Now().Unix())
			if stage == "publishing" {
				tx, _ := s.DB.Begin()
				Enqueue(tx, "jobs", "publish", v.CaseID, `{"digest":"saved"}`, time.Now().Unix())
				tx.Commit()
			}
			s.DB.Close()
			s, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.DB.Close()
			if err = s.Recover(ctx); err != nil {
				t.Fatal(err)
			}
			if err = s.Recover(ctx); err != nil {
				t.Fatal(err)
			}
			if err = s.Finish(ctx, "jobs", old, nil); err == nil {
				t.Fatal("old lease was not fenced")
			}
			var status, checkpoint string
			var cents int
			s.DB.QueryRow("SELECT status,reserved_cents FROM research_runs WHERE id=?", run).Scan(&status, &cents)
			s.DB.QueryRow("SELECT payload FROM research_checkpoints WHERE case_id=?", v.CaseID).Scan(&checkpoint)
			if status != "interrupted" || cents != 1 || checkpoint == "" {
				t.Fatal(status, cents, checkpoint)
			}
			job, err := s.Claim(ctx, "jobs", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if job.Kind == "research" && job.Attempts != 1 {
				t.Fatal("interruption consumed retry", job)
			}
			var n int
			s.DB.QueryRow("SELECT count(*) FROM jobs WHERE ref=?", v.CaseID).Scan(&n)
			want := 1
			if stage == "publishing" {
				want = 2
			}
			if n != want {
				t.Fatal(n, want)
			}
		})
	}
}
func TestRecoveryRepairsOrphanButPreservesBudgetAndTerminal(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	id := accept(t, s, false)
	v, _ := s.Get(ctx, id)
	s.DB.Exec("DELETE FROM jobs")
	s.Transition(ctx, v.CaseID, "drafted", "Saved draft before a missing enqueue.", "", "")
	if err := s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	j, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil || j.Kind != "research" {
		t.Fatal(j, err)
	}
	if err = s.Finish(ctx, "jobs", j, ErrBudget); err != nil {
		t.Fatal(err)
	}
	s.Recover(ctx)
	if _, err = s.Claim(ctx, "jobs", time.Minute); err == nil {
		t.Fatal("budget wait bypassed")
	}
	s.Transition(ctx, v.CaseID, "failed", "Explicit terminal failure.", "", "")
	s.Recover(ctx)
	v, _ = s.Get(ctx, id)
	if v.Status != "failed" {
		t.Fatal(v.Status)
	}
}
func TestExclusiveWorkerLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db.sqlite")
	a, err := LockWorker(p)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := LockWorker(p); err == nil {
		b.Close()
		t.Fatal("second worker accepted")
	}
	a.Close()
	b, err := LockWorker(p)
	if err != nil {
		t.Fatal(err)
	}
	b.Close()
}
func TestAgentLaunchFailureDoesNotExhaustCaseRetries(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	accept(t, s, false)
	if _, err := s.DB.Exec("UPDATE jobs SET attempts=7"); err != nil {
		t.Fatal(err)
	}
	job, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil || job.Attempts != 8 {
		t.Fatal(job.Attempts, err)
	}
	if err := s.Finish(ctx, "jobs", job, ErrResearchAgent); err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	if err := s.DB.QueryRow("SELECT status,attempts FROM jobs WHERE id=?", job.ID).Scan(&status, &attempts); err != nil || status != "queued" || attempts != 7 {
		t.Fatal(status, attempts, err)
	}
}

func TestGlobalSummaryIgnoresFiltersAndDeduplicatesCases(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	first := accept(t, s, false)
	legacyReceipt(t, s, first)
	j, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.ListSubmissions(ctx, "", "failed", "rss", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || page.Summary.Total != 2 || page.Summary.Running != 1 || page.Summary.Queued != 0 || len(page.Working) != 1 {
		t.Fatal(page)
	}
	s.Finish(ctx, "jobs", j, ErrInterrupted)
	page, err = s.ListSubmissions(ctx, "", "failed", "rss", "", "")
	if err != nil || page.Summary.Running != 0 || page.Summary.Queued != 1 {
		t.Fatal(page, err)
	}
}

func TestRecoveryOutboxRetainsPayloadAndFencesOldDelivery(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	tx, _ := s.DB.Begin()
	Enqueue(tx, "outbox", "resolution", "receipt:committed", `{"event":"same-signed-event"}`, time.Now().Unix())
	tx.Commit()
	old, err := s.Claim(ctx, "outbox", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := s.Claim(ctx, "outbox", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if next.ID != old.ID || next.Payload != old.Payload || next.Attempts != 1 || next.Token == old.Token {
		t.Fatal(old, next)
	}
	if err = s.Finish(ctx, "outbox", old, nil); err == nil {
		t.Fatal("old delivery was not fenced")
	}
}
func TestWorkingIncludesOldRestartOutsideHistoryPageAndFilter(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	id := accept(t, s, false)
	v, _ := s.Get(ctx, id)
	j, _ := s.Claim(ctx, "jobs", time.Minute)
	s.Transition(ctx, v.CaseID, "failed", "Previous technical failure.", "", "")
	s.Finish(ctx, "jobs", j, nil)
	s.DB.Exec("INSERT INTO research_checkpoints VALUES(?,?,?)", v.CaseID, `{"old":"discard"}`, time.Now().Unix())
	for i := 0; i < 55; i++ {
		url := "https://news.example/" + ID()
		if _, err := s.Accept(ctx, Intake{Origin: "rss", URL: url, CanonicalURL: url}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Restart(ctx, id); err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB.QueryRow("SELECT count(*) FROM research_checkpoints WHERE case_id=?", v.CaseID).Scan(&n)
	if n != 0 {
		t.Fatal("manual restart reused stale research")
	}
	// Claim the restarted job specifically, as if the worker has reached it.
	s.DB.Exec("UPDATE jobs SET status='running',lease_until=?,lease_token='fixture' WHERE ref=?", time.Now().Add(time.Minute).Unix(), v.CaseID)
	page, err := s.ListSubmissions(ctx, "", "failed", "rss", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Submissions) != 0 || len(page.Working) != 1 || page.Working[0].ID != id || page.Summary.Running != 1 || page.Summary.Total != 56 {
		t.Fatal(page)
	}
}

func TestRunningCountsOnlyJobsActuallyRunning(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	stale, _ := s.Accept(ctx, Intake{Origin: "web", URL: "https://news.example/stale", CanonicalURL: "https://news.example/stale"})
	live, _ := s.Accept(ctx, Intake{Origin: "web", URL: "https://news.example/live", CanonicalURL: "https://news.example/live"})
	a, _ := s.Get(ctx, stale)
	b, _ := s.Get(ctx, live)
	// a request left "researching" after a restart: its job is queued again, nothing runs it
	s.DB.Exec("UPDATE cases SET status='researching' WHERE id IN (?,?)", a.CaseID, b.CaseID)
	s.DB.Exec("UPDATE jobs SET status='running',lease_until=?,lease_token='x' WHERE ref=?", time.Now().Add(time.Minute).Unix(), b.CaseID)
	page, err := s.ListSubmissions(ctx, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Summary.Running != 1 || page.Summary.Queued != 1 || len(page.Working) != 1 || page.Working[0].ID != live {
		t.Fatal("a request with no running job counted as running", page.Summary, len(page.Working))
	}
}
