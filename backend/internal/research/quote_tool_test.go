package research

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// find_quote answers with the captured page's own wording of a paraphrased quotation.
func TestFindQuoteReturnsTheSourceWording(t *testing.T) {
	dir := t.TempDir()
	if err := SaveEvidence(dir, []safeurl.Evidence{{URL: "https://news.example/a", Content: "<p>The raid was on Monday.</p><p>Officers seized 1 quintal 45 kg of ganja and arrested five men.</p>"}}); err != nil {
		t.Fatal(err)
	}
	l := newReportLookup(dir)
	got := l.findQuote(json.RawMessage(`{"text":"officers seized 145 kg ganja, five arrested"}`))
	if !strings.Contains(got, "1. Officers seized 1 quintal 45 kg of ganja and arrested five men.") || !strings.Contains(got, "source: https://news.example/a") {
		t.Fatal(got)
	}
	if got := l.findQuote(json.RawMessage(`{"text":"officers seized ganja","source":"https://other.example/b"}`)); !strings.Contains(got, "No captured source with that URL") {
		t.Fatal(got)
	}
}

// Once half the steps are used, the check says how many are left, and urges the answer near the end.
func TestCheckSaysHowManyStepsAreLeft(t *testing.T) {
	s := &checkServer{maxSteps: 24}
	s.steps.Store(6)
	if s.stepNote(false) != "" {
		t.Fatal("no note before half the steps")
	}
	s.steps.Store(14)
	if n := s.stepNote(false); !strings.Contains(n, "10 left") || !strings.Contains(n, "one revision") {
		t.Fatal(n)
	}
	s.steps.Store(22)
	if n := s.stepNote(false); !strings.Contains(n, "only 2 left") || !strings.Contains(n, "final JSON answer now") {
		t.Fatal(n)
	}
}

// A run the step limit ends with notes instead of an answer keeps the last draft it checked.
func TestStepLimitKeepsTheLastCheckedDraft(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "runs.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	id, err := s.Accept(ctx, store.Intake{Origin: "web", URL: "https://news.example/a", CanonicalURL: "https://news.example/a"})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := s.Get(ctx, id)
	run, err := s.Reserve(ctx, v.CaseID, "", 100, 1000)
	if err != nil {
		t.Fatal(err)
	}
	s.StartRun(ctx, run, "provider/model", "local")
	script := filepath.Join(dir, "opencode")
	body := "#!/bin/sh\necho '{\"type\":\"step_start\",\"sessionID\":\"ses_1\"}'\necho '{\"type\":\"text\",\"part\":{\"text\":\"Maximum steps for this agent have been reached.\"}}'\necho '{\"type\":\"step_finish\",\"part\":{\"tokens\":{\"input\":1,\"output\":1}}}'\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	draft := `{"outcome":"not_relevant","explanation":"The article is about a traffic accident, not cannabis enforcement."}`
	r := Runner{Config: config.Config{ResearchRunner: "local", ResearchTimeout: time.Minute, ResearchMaxSteps: 24}, Store: s, tool: &checkServer{lastDraft: []byte(draft)}}
	got, err := r.execute(ctx, v.CaseID, dir, run, exec.CommandContext(ctx, script, "run", "x"), false)
	if err != nil || got.Outcome != "not_relevant" {
		t.Fatalf("%+v %v", got, err)
	}
	r.tool = &checkServer{}
	if _, err = r.execute(ctx, v.CaseID, dir, run, exec.CommandContext(ctx, script, "run", "x"), false); err == nil || !strings.Contains(err.Error(), "notes instead of the JSON") {
		t.Fatal("without a checked draft the run still fails", err)
	}
}
