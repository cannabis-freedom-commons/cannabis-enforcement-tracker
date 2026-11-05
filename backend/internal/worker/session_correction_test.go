package worker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// A rejected answer is corrected by continuing the same local OpenCode session in the same job:
// no requeue, one correction run, and the corrected answer passes through every check again.
func TestRejectedAnswerIsCorrectedInSameSession(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS local runner")
	}
	repo, _ := commitFixture(t)
	for _, name := range []string{"docs/CFI.md", "docs/WRITING.md", ".claude/skills/add-report/SKILL.md", ".claude/skills/find-reports/SKILL.md", ".opencode/agents/enforcement-researcher.md", "src/data/countries.json"} {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		content := "Fixture instructions"
		if strings.HasSuffix(name, "countries.json") {
			content = `[{"id":"it"}]`
		}
		if strings.HasSuffix(name, "enforcement-researcher.md") {
			content = "---\nmode: primary\npermission:\n  read: allow\n---\nFixture research instructions.\n"
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	original := "---\ncode: ABC123\ncountry: it\ndate: 2026-11-03\ntitle: Police arrest two men\ncreated_by_model: original/model\nstatus: reported\nsources:\n  - url: https://first.example/article\n    title: First account\n    publisher: First\n---\n\nPolice arrested two men at the farm. [@1]\n"
	path := filepath.Join(repo, "reports/2026/11/existing-case.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "add", ".")
	testGit(t, repo, "commit", "-m", "Fixture inputs")
	s, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ctx := context.Background()
	url := "https://second.example/article"
	id, err := s.Accept(ctx, store.Intake{Origin: "web", URL: url, CanonicalURL: url})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := s.Get(ctx, id)
	sources := []research.Source{{URL: url, Title: "Second account", Publisher: "Second"}}
	// The first answer cites a source that does not exist; the correction is a supported duplicate.
	bad := research.Result{Outcome: "additional_source", ExistingReport: "existing-case", Explanation: "Another opened account corroborates the recorded farm arrests.", Sources: sources, Facts: []research.Fact{{Text: "Two suspects were detained at this farm.", Source: 2, Quote: "Police arrested two men at the farm."}}}
	good := research.Result{Outcome: "duplicate", ExistingReport: "existing-case", Explanation: "The original report already covers the farm arrests and the supported facts.", Sources: sources}
	initial := safeurl.Evidence{URL: url, FinalURL: url, Content: bad.Facts[0].Quote}
	// A saved coverage review skips the source crawl, so the fixture needs no network.
	saved := researchCheckpoint{Result: bad, Initial: initial, Evidence: []safeurl.Evidence{initial}, Model: "fixture/model", Dir: "missing-old-checkout", ReviewCoverage: true}
	if _, err := s.DB.Exec("INSERT INTO research_checkpoints VALUES(?,?,?)", v.CaseID, store.JSON(saved), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	answer := func(r research.Result) string {
		return store.JSON(map[string]any{"type": "text", "part": map[string]string{"text": store.JSON(r)}})
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 1.0.0; exit 0; fi\nfor a in \"$@\"; do if [ \"$a\" = --continue ]; then cat <<'RESPONSE'\n" + answer(good) + "\nRESPONSE\nexit 0; fi; done\ncat <<'RESPONSE'\n" + answer(bad) + "\nRESPONSE\n"
	binary := filepath.Join(bin, "opencode")
	if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	providerConfig := filepath.Join(bin, "opencode.json")
	if err := os.WriteFile(providerConfig, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	w := Worker{Store: s, Config: config.Config{RepoPath: repo, Workspaces: t.TempDir(), ResearchRunner: "local", OpenCodeBinary: binary, OpenCodeConfigFile: providerConfig, OpenCodeAuthFile: filepath.Join(bin, "missing-auth.json"), ResearchModel: "fixture/model", RunCents: 1}}
	job, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.process(ctx, job); err != nil {
		t.Fatal("in-session correction did not finish", err)
	}
	v, err = s.Get(ctx, id)
	if err != nil || v.Status != "duplicate" {
		t.Fatal("corrected answer was not used", v.Status, err)
	}
	var runs int
	if err := s.DB.QueryRow("SELECT count(*) FROM research_runs WHERE case_id=? AND status='completed'", v.CaseID).Scan(&runs); err != nil || runs != 2 {
		t.Fatal("expected the research run and one correction run", runs, err)
	}
	var queued int
	if err := s.DB.QueryRow("SELECT count(*) FROM submission_history WHERE status='queued' AND explanation LIKE '%needs a correction%'").Scan(&queued); err != nil || queued != 0 {
		t.Fatal("the correction went back to the queue", queued, err)
	}
	var payload string
	if err := s.DB.QueryRow("SELECT payload FROM research_checkpoints WHERE case_id=?", v.CaseID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	saved = researchCheckpoint{}
	if err := json.Unmarshal([]byte(payload), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.RepairAttempts != 1 || saved.ReviewCoverage || saved.Result.Outcome != "duplicate" {
		t.Fatal("correction budget or checkpoint wrong", saved)
	}
	if actual, err := os.ReadFile(path); err != nil || string(actual) != original {
		t.Fatal("original report changed", err)
	}
}

func TestInterruptedRunResumesSessionAndUsesCheckTool(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS local runner")
	}
	repo, _ := commitFixture(t)
	for _, name := range []string{"docs/CFI.md", "docs/WRITING.md", ".claude/skills/add-report/SKILL.md", ".claude/skills/find-reports/SKILL.md", ".opencode/agents/enforcement-researcher.md", "src/data/countries.json"} {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		content := "Fixture instructions"
		if strings.HasSuffix(name, "countries.json") {
			content = `[{"id":"it"}]`
		}
		if strings.HasSuffix(name, "enforcement-researcher.md") {
			content = "---\nmode: primary\npermission:\n  read: allow\n---\nFixture research instructions.\n"
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	original := "---\ncode: ABC123\ncountry: it\ndate: 2026-11-03\ntitle: Police arrest two men\ncreated_by_model: original/model\nstatus: reported\nsources:\n  - url: https://first.example/article\n    title: First account\n    publisher: First\n---\n\nPolice arrested two men at the farm. [@1]\n"
	path := filepath.Join(repo, "reports/2026/11/existing-case.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "add", ".")
	testGit(t, repo, "commit", "-m", "Fixture inputs")
	s, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ctx := context.Background()
	url := "https://second.example/article"
	id, err := s.Accept(ctx, store.Intake{Origin: "web", URL: url, CanonicalURL: url})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := s.Get(ctx, id)
	sources := []research.Source{{URL: url, Title: "Second account", Publisher: "Second"}}
	// The first answer cites a source that does not exist; the correction is a supported duplicate.
	bad := research.Result{Outcome: "additional_source", ExistingReport: "existing-case", Explanation: "Another opened account corroborates the recorded farm arrests.", Sources: sources, Facts: []research.Fact{{Text: "Two suspects were detained at this farm.", Source: 2, Quote: "Police arrested two men at the farm."}}}
	good := research.Result{Outcome: "duplicate", ExistingReport: "existing-case", Explanation: "The original report already covers the farm arrests and the supported facts.", Sources: sources}
	initial := safeurl.Evidence{URL: url, FinalURL: url, Content: bad.Facts[0].Quote}
	// A saved coverage review skips the source crawl, so the fixture needs no network.
	saved := researchCheckpoint{Result: bad, Initial: initial, Evidence: []safeurl.Evidence{initial}, Model: "fixture/model", Dir: "missing-old-checkout", ReviewCoverage: true}
	if _, err := s.DB.Exec("INSERT INTO research_checkpoints VALUES(?,?,?)", v.CaseID, store.JSON(saved), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	answer := func(r research.Result) string {
		return store.JSON(map[string]any{"type": "text", "part": map[string]string{"text": store.JSON(r)}})
	}
	bin := t.TempDir()
	// First attempt: one model step, then the process dies. Resumed attempt: ask the real check
	// tool about the bad answer and return the good one only if the tool rejected it.
	step := `{"type":"step_finish","part":{"tokens":{"input":1,"output":1}}}`
	call := store.JSON(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "check_result", "arguments": map[string]any{"result": bad}}})
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 1.0.0; exit 0; fi\nfor a in \"$@\"; do if [ \"$a\" = --continue ]; then\n" +
		"url=$(grep -o '\"url\":\"[^\"]*' opencode.json | cut -d'\"' -f4)\n" +
		"auth=$(grep -o '\"Authorization\":\"[^\"]*' opencode.json | cut -d'\"' -f4)\n" +
		"reply=$(/usr/bin/curl -s -H \"Authorization: $auth\" -H 'Content-Type: application/json' --data-binary @- \"$url\" <<'CALL'\n" + call + "\nCALL\n)\n" +
		"case \"$reply\" in *'would reject'*) cat <<'RESPONSE'\n" + answer(good) + "\nRESPONSE\n;; *) cat <<'RESPONSE'\n" + answer(bad) + "\nRESPONSE\n;; esac\nexit 0; fi; done\n" +
		"echo '" + step + "'\nexit 1\n"
	binary := filepath.Join(bin, "opencode")
	if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	providerConfig := filepath.Join(bin, "opencode.json")
	if err := os.WriteFile(providerConfig, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	w := Worker{Store: s, Config: config.Config{RepoPath: repo, Workspaces: t.TempDir(), ResearchRunner: "local", OpenCodeBinary: binary, OpenCodeConfigFile: providerConfig, OpenCodeAuthFile: filepath.Join(bin, "missing-auth.json"), ResearchModel: "fixture/model", RunCents: 1}}
	job, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.process(ctx, job); err == nil {
		t.Fatal("the interrupted run did not fail")
	}
	root, _ := filepath.Abs(w.Config.Workspaces)
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 || !research.HasSession(filepath.Join(root, entries[0].Name())) {
		t.Fatal("interrupted session was not kept", len(entries))
	}
	first := filepath.Join(root, entries[0].Name())
	if err := w.cleanWorkspaces(ctx, v.CaseID); err != nil || !research.HasSession(first) {
		t.Fatal("cleanup removed the session waiting to be resumed", err)
	}
	if err := s.Finish(ctx, "jobs", job, errors.New("interrupted")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("UPDATE jobs SET available_at=0,status='queued'"); err != nil {
		t.Fatal(err)
	}
	next, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.process(ctx, next); err != nil {
		t.Fatal("resumed run did not finish", err)
	}
	v, err = s.Get(ctx, id)
	if err != nil || v.Status != "duplicate" {
		t.Fatal("the check tool's feedback was not used", v.Status, err)
	}
	var workspaces int
	if err := s.DB.QueryRow("SELECT count(DISTINCT workspace) FROM research_runs WHERE case_id=? AND workspace<>''", v.CaseID).Scan(&workspaces); err != nil || workspaces != 1 {
		t.Fatal("the resume did not reuse the interrupted workspace", workspaces, err)
	}
	if research.HasSession(first) {
		t.Fatal("finished session was not removed")
	}
	var payload string
	if err := s.DB.QueryRow("SELECT payload FROM research_checkpoints WHERE case_id=?", v.CaseID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	saved = researchCheckpoint{}
	if err := json.Unmarshal([]byte(payload), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.RepairAttempts != 0 || saved.ReviewCoverage || saved.Result.Outcome != "duplicate" {
		t.Fatal("the check tool should have avoided a correction", saved)
	}
	if actual, err := os.ReadFile(path); err != nil || string(actual) != original {
		t.Fatal("original report changed", err)
	}
}
