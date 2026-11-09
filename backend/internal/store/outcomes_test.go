package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestOutcomesCountStatusesOriginsAndTimeToCommit(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ctx := context.Background()
	a, _ := s.Accept(ctx, Intake{Origin: "web", URL: "https://news.example/a", CanonicalURL: "https://news.example/a"})
	s.Accept(ctx, Intake{Origin: "rss", URL: "https://news.example/b", CanonicalURL: "https://news.example/b"})
	v, _ := s.Get(ctx, a)
	now := time.Now().Unix()
	s.DB.Exec("UPDATE submissions SET created_at=? WHERE id=?", now-90*60, a)
	s.DB.Exec("INSERT INTO local_publications(case_id,commit_sha,report_path,created_at) VALUES(?,?,?,?)", v.CaseID, "sha", "reports/a.md", now)
	s.DB.Exec("UPDATE cases SET status='published',report_slug='a' WHERE id=?", v.CaseID)
	s.DB.Exec("INSERT INTO submission_history(submission_id,status,explanation,actor,created_at) VALUES(?,?,?,?,?)", a, "published", "On the site.", "worker", now)
	out, err := s.Outcomes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out.ByStatus["published"] != 1 || out.ByStatus["queued"] != 1 || out.ByOrigin["web"] != 1 || out.ByOrigin["rss"] != 1 {
		t.Fatal("counts", out.ByStatus, out.ByOrigin)
	}
	if out.Received.Week != 2 || out.Committed.Day != 1 || out.Published.Week != 1 {
		t.Fatal("windows", out.Received, out.Committed, out.Published)
	}
	if out.MinutesToCommit == nil || out.MinutesToCommit.Median != 90 {
		t.Fatal("time to commit", out.MinutesToCommit)
	}
}
