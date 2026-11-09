package worker

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOverlapReasonKeepsModelAssessmentAndPublicLinks(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "decisions.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ctx := context.Background()
	old, err := s.Accept(ctx, store.Intake{Origin: "web", URL: "https://news.example/old", CanonicalURL: "https://news.example/old"})
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.Accept(ctx, store.Intake{Origin: "web", URL: "https://news.example/new", CanonicalURL: "https://news.example/new"})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.Get(ctx, old)
	b, _ := s.Get(ctx, current)
	if err = s.Transition(ctx, a.CaseID, "duplicate", "Already covered by an existing report.", "existing-report", ""); err != nil {
		t.Fatal(err)
	}
	assessment := `{"explanation":"The article supports a new report.","decision_title":"A supported sentencing event"}`
	s.SetCaseOutput(ctx, b.CaseID, map[string]string{"ai_result": assessment})
	w := Worker{Store: s}
	if err = w.backend().RejectSourceOverlap(ctx, b.CaseID, a.CaseID, "https://news.example/shared", research.Validation{Passed: true}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	var validation struct {
		SourceOverlap struct {
			URL          string `json:"url"`
			SubmissionID string `json:"submission_id"`
			ReportSlug   string `json:"report_slug"`
		} `json:"source_overlap"`
	}
	if err = json.Unmarshal(got.Validation, &validation); err != nil {
		t.Fatal(err)
	}
	if got.Status != "insufficient_evidence" || len(got.Explanation) < 200 || !strings.Contains(got.Explanation, "service") || validation.SourceOverlap.SubmissionID != old || validation.SourceOverlap.ReportSlug != "existing-report" || string(got.Result) != assessment {
		t.Fatal(got, validation)
	}
}

func TestSourceOfACaseWithoutAReportPassesOn(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "claims.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ctx := context.Background()
	ids := []string{}
	for _, u := range []string{"https://news.example/a", "https://news.example/b", "https://news.example/c"} {
		id, err := s.Accept(ctx, store.Intake{Origin: "web", URL: u, CanonicalURL: u})
		if err != nil {
			t.Fatal(err)
		}
		v, _ := s.Get(ctx, id)
		ids = append(ids, v.CaseID)
	}
	w := Worker{Store: s}
	shared := "https://news.example/shared"
	// a queued case recorded the source first: it holds nothing, so the next case takes the source
	if owner, err := w.backend().ClaimSource(ctx, ids[0], shared, shared, "d"); err != nil || owner != ids[0] {
		t.Fatal(owner, err)
	}
	if owner, err := w.backend().ClaimSource(ctx, ids[1], shared, shared, "d"); err != nil || owner != ids[1] {
		t.Fatal("a queued case blocked the source", owner, err)
	}
	// a case that made a report from it keeps it
	if _, err = s.DB.Exec("UPDATE cases SET status='committed',report_slug='a-report' WHERE id=?", ids[1]); err != nil {
		t.Fatal(err)
	}
	if owner, err := w.backend().ClaimSource(ctx, ids[2], shared, shared, "d"); err != nil || owner != ids[1] {
		t.Fatal("a committed case lost its source", owner, err)
	}
}

func TestFailedAttemptWaitingToRetryShowsAsQueued(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "retry.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ctx := context.Background()
	id, _ := s.Accept(ctx, store.Intake{Origin: "web", URL: "https://news.example/a", CanonicalURL: "https://news.example/a"})
	v, _ := s.Get(ctx, id)
	j, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	s.Transition(ctx, v.CaseID, "researching", "OpenCode is researching.", "", "")
	cause := errors.New("OpenCode exited with code 1")
	if err = s.Finish(ctx, "jobs", j, cause); err != nil {
		t.Fatal(err)
	}
	(&Worker{Store: s}).waitingToRetry(ctx, j, cause)
	if v, _ = s.Get(ctx, id); v.Status != "queued" || !strings.Contains(v.Explanation, "retried automatically") {
		t.Fatal("a request waiting to retry still shows as researching", v.Status, v.Explanation)
	}
}
