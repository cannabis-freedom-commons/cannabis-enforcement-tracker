package worker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func TestInvalidResultGetsOneEvidencePreservingCorrection(t *testing.T) {
	for _, failure := range []string{"schema", "merge"} {
		t.Run(failure, func(t *testing.T) {
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
			result := research.Result{Outcome: "additional_source", ExistingReport: "existing-case", Explanation: "Another opened account corroborates the recorded farm arrests.", Sources: []research.Source{{URL: url, Title: "Second account", Publisher: "Second"}}, Facts: []research.Fact{{Text: "Two suspects were detained at this farm.", Source: 1, Quote: "Police arrested two men at the farm."}}}
			if failure == "schema" {
				result.Facts[0].Source = 2
			}
			initial := safeurl.Evidence{URL: url, FinalURL: url, Content: result.Facts[0].Quote}
			saved := researchCheckpoint{Result: result, Initial: initial, Evidence: []safeurl.Evidence{initial}, Model: "original/model", Dir: "missing-old-checkout"}
			if _, err := s.DB.Exec("INSERT INTO research_checkpoints VALUES(?,?,?)", v.CaseID, store.JSON(saved), time.Now().Unix()); err != nil {
				t.Fatal(err)
			}
			job, err := s.Claim(ctx, "jobs", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			w := Worker{Store: s, Config: config.Config{RepoPath: repo, Workspaces: t.TempDir()}}
			if err := w.process(ctx, job); !errors.Is(err, errCoverageRequeued) {
				t.Fatal("did not request a correction", err)
			}
			if err := s.Finish(ctx, "jobs", job, nil); err == nil {
				t.Fatal("replaced job lease was not fenced")
			}
			var payload string
			if err := s.DB.QueryRow("SELECT payload FROM research_checkpoints WHERE case_id=?", v.CaseID).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			saved = researchCheckpoint{}
			if err := json.Unmarshal([]byte(payload), &saved); err != nil {
				t.Fatal(err)
			}
			if !saved.ReviewCoverage || saved.ReviewProblem == "" || saved.RepairAttempts != 1 || saved.Initial.Content != initial.Content || len(saved.Evidence) != 1 {
				t.Fatal("lost evidence or correction bounds", saved)
			}
			// A fake harness corrects the merge outcome, while repeating the schema
			// error proves that the correction cannot keep requeuing itself.
			answer := result
			want := "failed"
			if failure == "merge" {
				answer = research.Result{Outcome: "duplicate", ExistingReport: "existing-case", Explanation: "The original report already covers the farm arrests and the supported facts.", Sources: result.Sources}
				want = "duplicate"
			}
			bin := t.TempDir()
			event := store.JSON(map[string]any{"type": "text", "part": map[string]string{"text": store.JSON(answer)}})
			script := "#!/bin/sh\nif [ \"$1\" = run ]; then\ncat <<'RESPONSE'\n" + event + "\nRESPONSE\nfi\n"
			if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			w.Config.ResearchRunner, w.Config.ResearchImage, w.Config.ResearchKey, w.Config.ResearchModel = "docker", "fixture-image", "fixture-key", "fixture/model"
			w.Config.RunCents = 1
			next, err := s.Claim(ctx, "jobs", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if err := w.process(ctx, next); err != nil {
				t.Fatal("correction did not finish", err)
			}
			v, err = s.Get(ctx, id)
			if err != nil || v.Status != want {
				t.Fatal(v.Status, want, err)
			}
			var workspace string
			if err := s.DB.QueryRow("SELECT workspace FROM research_runs WHERE case_id=?", v.CaseID).Scan(&workspace); err != nil {
				t.Fatal(err)
			}
			request, err := os.ReadFile(filepath.Join(workspace, ".research-input/request.json"))
			if err != nil || !strings.Contains(string(request), `"validation_feedback":`) {
				t.Fatal("model did not receive the correction", err, string(request))
			}
			if err := s.DB.QueryRow("SELECT payload FROM research_checkpoints WHERE case_id=?", v.CaseID).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			saved = researchCheckpoint{}
			if err := json.Unmarshal([]byte(payload), &saved); err != nil {
				t.Fatal(err)
			}
			if saved.RepairAttempts != 1 || saved.ReviewCoverage {
				t.Fatal("correction budget reset", saved)
			}
			if actual, err := os.ReadFile(path); err != nil || string(actual) != original {
				t.Fatal("unsafe merge changed original prose", err)
			}
		})
	}
}

func TestCaptureStorageErrorsAreNotPublisherFailures(t *testing.T) {
	for _, message := range []string{"mkdir sources: no space left on device", "ENOSPC: write failed"} {
		if !captureStorageError(errors.New(message)) {
			t.Fatal(message)
		}
	}
	if captureStorageError(nil) || captureStorageError(errors.New("The browser page answered HTTP 403.")) {
		t.Fatal("confused a publisher error with local storage")
	}
}
