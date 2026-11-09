package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func eventDraft(t *testing.T, name, code, country string) draftJob {
	t.Helper()
	dir := t.TempDir()
	rel := "reports/2026/11/" + name + ".md"
	content := "---\ncode: \"" + code + "\"\ncountry: " + country + "\ndate: 2026-11-03\nadded_at: \"2026-11-03T16:00:00Z\"\ntitle: Pozzuoli greenhouse raid at Monte Nuovo\nsources:\n  - url: https://" + name + ".example/article\n---\nPolice arrested a man. [@1]\n"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	return draftJob{Dir: dir, Rel: rel, Slug: name, Title: "A report", Digest: hex.EncodeToString(sum[:]), Coverage: map[string]string{}}
}

func TestConcurrentNewReportsRecheckCoverageUnderCommitLock(t *testing.T) {
	for _, tc := range []struct {
		name, country string
		distinctTitle bool
		want          int
	}{
		{"possible match", "it", false, 1},
		{"unrelated report in same country", "it", true, 2},
		{"unrelated country", "gb", false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := commitFixture(t)
			os.MkdirAll(filepath.Join(repo, "reports"), 0755)
			a := eventDraft(t, "first-outlet", "ABC123", "it")
			b := eventDraft(t, "second-outlet", "XYZ123", tc.country)
			if tc.distinctTitle {
				path := filepath.Join(b.Dir, b.Rel)
				original, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				content := []byte(strings.Replace(string(original), "Pozzuoli greenhouse raid at Monte Nuovo", "Court orders demolition of warehouse", 1))
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(content)
				b.Digest = hex.EncodeToString(digest[:])
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			for i, d := range []draftJob{a, b} {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					_, err := commitReport(context.Background(), repo, []string{"first", "second"}[i], d)
					results <- err
				}()
			}
			close(start)
			wg.Wait()
			close(results)
			committed, stale := 0, 0
			for err := range results {
				var changed *coverageChanged
				switch {
				case err == nil:
					committed++
				case errors.As(err, &changed):
					stale++
				default:
					t.Fatal(err)
				}
			}
			if committed != tc.want || stale != 2-tc.want {
				t.Fatal("stale coverage was published", committed, stale)
			}
			reports, err := Coverage(repo)
			if err != nil || len(reports) != tc.want {
				t.Fatal(reports, err)
			}
		})
	}
}

func TestCoverageInventorySurvivesFreshRecoveryCheckout(t *testing.T) {
	repo, _ := commitFixture(t)
	os.MkdirAll(filepath.Join(repo, "reports"), 0755)
	a := eventDraft(t, "first", "ABC123", "it")
	if _, err := commitReport(context.Background(), repo, "first-case", a); err != nil {
		t.Fatal(err)
	}
	b := eventDraft(t, "second", "XYZ123", "it")
	// Recovery's fresh checkout already contains the first report. Its inventory
	// must still describe what the model saw, not what recovery copied later.
	data, _ := os.ReadFile(filepath.Join(repo, a.Rel))
	os.WriteFile(filepath.Join(b.Dir, a.Rel), data, 0644)
	encoded := store.JSON(b)
	json.Unmarshal([]byte(encoded), &b)
	var changed *coverageChanged
	if _, err := commitReport(context.Background(), repo, "second-case", b); !errors.As(err, &changed) {
		t.Fatal("fresh checkout bypassed original coverage", err)
	}
}

func TestCoverageReviewHandoffPreservesEvidenceAndFencesPublication(t *testing.T) {
	repo, _ := commitFixture(t)
	os.MkdirAll(filepath.Join(repo, "reports"), 0755)
	a := eventDraft(t, "first", "ABC123", "it")
	if _, err := commitReport(context.Background(), repo, "first-case", a); err != nil {
		t.Fatal(err)
	}
	d := eventDraft(t, "second", "XYZ123", "it")
	s, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ctx := context.Background()
	id, _ := s.Accept(ctx, store.Intake{Origin: "web", URL: "https://second.example/article", CanonicalURL: "https://second.example/article"})
	v, _ := s.Get(ctx, id)
	old, _ := s.Claim(ctx, "jobs", time.Minute)
	s.Finish(ctx, "jobs", old, nil)
	corroboration := strings.Repeat("Saved corroboration ", 400) + "END-CAPTURE"
	saved := researchCheckpoint{Result: research.Result{Outcome: "new_event", Country: "it"}, Model: "original/model", Initial: safeurl.Evidence{URL: "https://second.example/article", Content: "Captured source"}, Evidence: []safeurl.Evidence{{URL: "https://corroboration.example/article", Content: corroboration}}, Coverage: map[string]string{}}
	s.DB.Exec("INSERT INTO research_checkpoints VALUES(?,?,?)", v.CaseID, store.JSON(saved), time.Now().Unix())
	tx, _ := s.DB.Begin()
	store.Enqueue(tx, "jobs", "publish", v.CaseID, store.JSON(d), time.Now().Unix())
	tx.Commit()
	j, _ := s.Claim(ctx, "jobs", time.Minute)
	w := Worker{Store: s, Config: config.Config{RepoPath: repo, Workspaces: filepath.Dir(d.Dir)}}
	if err = w.publish(ctx, j); !errors.Is(err, errCoverageRequeued) {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(repo, d.Rel)); !os.IsNotExist(err) {
		t.Fatal("stale report was written", err)
	}
	if err = s.Finish(ctx, "jobs", j, nil); err == nil {
		t.Fatal("replaced publication lease remained valid")
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	var payload string
	s.DB.QueryRow("SELECT payload FROM research_checkpoints WHERE case_id=?", v.CaseID).Scan(&payload)
	json.Unmarshal([]byte(payload), &saved)
	if !saved.ReviewCoverage || saved.Initial.Content != "Captured source" || len(saved.Evidence) != 1 || saved.Evidence[0].Content != corroboration || saved.Model != "original/model" || saved.AddedAt != "2026-11-03T16:00:00Z" {
		t.Fatal("lost research during handoff", payload)
	}
	next, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil || next.Kind != "research" || next.Ref != v.CaseID {
		t.Fatal(next, err)
	}
	if err = w.process(ctx, next); !errors.Is(err, store.ErrConfiguration) {
		t.Fatal("recovery reused the stale proposal without a new identity check", err)
	}
	v, _ = s.Get(ctx, id)
	if !strings.Contains(v.Explanation, "latest reports") {
		t.Fatal(v)
	}
	// Exercise the complete review path with a fake harness process. It must use
	// saved captures and the latest report, then finish as a linked duplicate.
	for _, name := range []string{"docs/CFI.md", "docs/WRITING.md", ".claude/skills/add-report/SKILL.md", ".claude/skills/find-reports/SKILL.md", ".opencode/agents/enforcement-researcher.md", "src/data/countries.json"} {
		path := filepath.Join(repo, name)
		os.MkdirAll(filepath.Dir(path), 0755)
		content := "Fixture instructions"
		if strings.HasSuffix(name, "countries.json") {
			content = `[{"id":"it"}]`
		}
		if strings.HasSuffix(name, "enforcement-researcher.md") {
			content = "---\nmode: primary\npermission:\n  read: allow\n---\nFixture research instructions.\n"
		}
		os.WriteFile(path, []byte(content), 0644)
	}
	bin := t.TempDir()
	answer := store.JSON(map[string]any{"type": "text", "part": map[string]string{"text": store.JSON(research.Result{Outcome: "duplicate", ExistingReport: "first", Explanation: "The first report already covers this incident and the available facts.", Sources: []research.Source{{URL: "https://second.example/article", Title: "Second account", Publisher: "Second outlet"}}})}})
	script := "#!/bin/sh\nif [ \"$1\" = run ]; then\ncat <<'RESPONSE'\n" + answer + "\nRESPONSE\nfi\n"
	os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	w.Config.ResearchRunner, w.Config.ResearchImage, w.Config.ResearchKey, w.Config.ResearchModel = "docker", "fixture-image", "fixture-key", "fixture/model"
	w.Config.RunCents = 1
	if err = w.process(ctx, next); err != nil {
		t.Fatal("coverage review did not finish", err)
	}
	v, _ = s.Get(ctx, id)
	if v.Status != "duplicate" || v.ReportSlug != "first" {
		t.Fatal("review lost the matching report link", v)
	}
	var workspace string
	s.DB.QueryRow("SELECT workspace FROM research_runs WHERE case_id=?", v.CaseID).Scan(&workspace)
	input := filepath.Join(workspace, ".research-input")
	request, err := os.ReadFile(filepath.Join(input, "request.json"))
	if err != nil || !strings.Contains(string(request), `"coverage_review":true`) {
		t.Fatal("missing review instruction", string(request), err)
	}
	index, _ := os.ReadFile(filepath.Join(input, "reports-index.jsonl"))
	if !strings.Contains(string(index), `"slug":"first"`) {
		t.Fatal("review did not see the current report", string(index))
	}
	captured, err := os.ReadFile(filepath.Join(input, "saved-sources/1.txt"))
	if err != nil || !strings.Contains(string(captured), "END-CAPTURE") {
		t.Fatal("lost saved evidence", err)
	}
	for _, line := range strings.Split(string(captured), "\n") {
		if len(line) > 1000 {
			t.Fatal("saved capture would be truncated by the model's read tool")
		}
	}
}

func TestRedirectedSourceAliasesKeepOneOwner(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	ctx := context.Background()
	var cases []string
	for _, url := range []string{"https://outlet.example/old-path", "https://outlet.example/other-path"} {
		id, _ := s.Accept(ctx, store.Intake{Origin: "rss", URL: url, CanonicalURL: url})
		v, _ := s.Get(ctx, id)
		cases = append(cases, v.CaseID)
	}
	w := Worker{Store: s}
	shared := "https://outlet.example/canonical-article"
	s.Transition(ctx, cases[0], "verifying", "Checking the first source.", "", "")
	if owner, err := w.backend().ClaimSource(ctx, cases[0], "https://outlet.example/old-path", shared, "first"); err != nil || owner != cases[0] {
		t.Fatal(owner, err)
	}
	s.Transition(ctx, cases[1], "verifying", "Checking the second source.", "", "")
	if owner, err := w.backend().ClaimSource(ctx, cases[1], "https://outlet.example/other-path", shared+"?utm_source=bing", "second"); err != nil || owner != cases[0] {
		t.Fatal("redirect alias bypassed the active owner", owner, err)
	}
	s.Transition(ctx, cases[0], "failed", "The first proposal did not validate.", "", "")
	if owner, err := w.backend().ClaimSource(ctx, cases[1], "https://outlet.example/other-path", shared, "second"); err != nil || owner != cases[1] {
		t.Fatal("failed owner blocked a valid retry", owner, err)
	}
}
