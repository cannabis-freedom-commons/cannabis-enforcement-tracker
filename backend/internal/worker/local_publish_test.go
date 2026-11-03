package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, data)
	}
	return strings.TrimSpace(string(data))
}
func commitFixture(t *testing.T) (string, draftJob) {
	t.Helper()
	repo := t.TempDir()
	testGit(t, repo, "init")
	testGit(t, repo, "config", "user.name", "Fixture")
	testGit(t, repo, "config", "user.email", "fixture@localhost")
	os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("original"), 0644)
	testGit(t, repo, "add", ".")
	testGit(t, repo, "commit", "-m", "Initial")
	root := t.TempDir()
	workspace := filepath.Join(root, "job")
	os.MkdirAll(filepath.Join(workspace, "reports/2026/10"), 0755)
	path := "reports/2026/10/fixture-report.md"
	content := "---\ncode: 7KQ2MX\nopen_questions:\n  - Which vegetables were in the truck?\n---\nCited report.\n"
	os.WriteFile(filepath.Join(workspace, path), []byte(content), 0644)
	digest := sha256.Sum256([]byte(content))
	return repo, draftJob{Dir: workspace, Rel: path, Slug: "fixture-report", Title: "Fixture report", Digest: hex.EncodeToString(digest[:])}
}
func TestLocalCommitPreservesOtherStagedAndWorkingFilesAndRecoversRetry(t *testing.T) {
	repo, d := commitFixture(t)
	os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("staged"), 0644)
	testGit(t, repo, "add", "unrelated.txt")
	os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("unstaged"), 0644)
	os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("untracked"), 0644)
	before := testGit(t, repo, "status", "--porcelain")
	sha, err := commitReport(context.Background(), repo, "case-fixture", d)
	if err != nil {
		t.Fatal(err)
	}
	if got := testGit(t, repo, "diff-tree", "--no-commit-id", "--name-only", "-r", sha); got != d.Rel {
		t.Fatal("committed unrelated files:", got)
	}
	if after := testGit(t, repo, "status", "--porcelain"); after != before {
		t.Fatalf("changed user work: %q => %q", before, after)
	}
	if got := testGit(t, repo, "show", "HEAD:unrelated.txt"); got != "original" {
		t.Fatal(got)
	}
	if got := testGit(t, repo, "show", ":unrelated.txt"); got != "staged" {
		t.Fatal(got)
	}
	again, err := commitReport(context.Background(), repo, "case-fixture", d)
	if err != nil || again != sha {
		t.Fatal("retry duplicated commit", again, err)
	}
}
func TestLocalCommitRefusesExistingUntrackedReport(t *testing.T) {
	repo, d := commitFixture(t)
	target := filepath.Join(repo, d.Rel)
	os.MkdirAll(filepath.Dir(target), 0755)
	os.WriteFile(target, []byte("someone else's work"), 0644)
	if _, err := commitReport(context.Background(), repo, "case-fixture", d); err == nil {
		t.Fatal("overwrote untracked report")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "someone else's work" {
		t.Fatal("changed existing report")
	}
}
func TestPublishJobCommitsWithoutGitHubAndIsIdempotent(t *testing.T) {
	repo, d := commitFixture(t)
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "store.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	id, err := s.Accept(ctx, store.Intake{Origin: "web", URL: "https://news.example/report", CanonicalURL: "https://news.example/report"})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := s.Get(ctx, id)
	// Finish intake research so the publication job is next.
	research, _ := s.Claim(ctx, "jobs", time.Minute)
	s.Finish(ctx, "jobs", research, nil)
	tx, _ := s.DB.BeginTx(ctx, nil)
	if err = store.Enqueue(tx, "jobs", "publish", v.CaseID, store.JSON(d), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	job, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	w := Worker{Store: s, Config: config.Config{RepoPath: repo, Workspaces: filepath.Dir(d.Dir), AutoPublish: true}}
	if err = w.publish(ctx, job); err != nil {
		t.Fatal(err)
	}
	result, err := s.Get(ctx, id)
	if err != nil || result.Status != "committed" || result.PRURL != "" || result.ReportSlug != d.Slug {
		t.Fatal(result, err)
	}
	if err = w.publish(ctx, job); err != nil {
		t.Fatal("completed job retry failed", err)
	}
	if count := testGit(t, repo, "rev-list", "--count", "HEAD"); count != "2" {
		t.Fatal("duplicate commit", count)
	}
}

func TestLocalCommitMovesExistingReportAndRefusesConcurrentEdits(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "move", true: "concurrent edit"}[changed], func(t *testing.T) {
			repo, d := commitFixture(t)
			old := "reports/2026/09/fixture-report.md"
			os.MkdirAll(filepath.Dir(filepath.Join(repo, old)), 0755)
			os.WriteFile(filepath.Join(repo, old), []byte("original report"), 0644)
			testGit(t, repo, "add", old)
			testGit(t, repo, "commit", "-m", "Original report")
			snapshot := filepath.Join(t.TempDir(), "snapshot")
			testGit(t, repo, "clone", "--local", repo, snapshot)
			content, _ := os.ReadFile(filepath.Join(d.Dir, d.Rel))
			d.Dir = snapshot
			d.PreviousRel = old
			os.MkdirAll(filepath.Dir(filepath.Join(snapshot, d.Rel)), 0755)
			os.WriteFile(filepath.Join(snapshot, d.Rel), content, 0644)
			os.Remove(filepath.Join(snapshot, old))
			if changed {
				os.WriteFile(filepath.Join(repo, old), []byte("newer report"), 0644)
				testGit(t, repo, "add", old)
				testGit(t, repo, "commit", "-m", "Concurrent update")
			}
			sha, err := commitReport(context.Background(), repo, "case-move", d)
			if changed {
				if err == nil {
					t.Fatal("replaced newer report")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(repo, old)); !os.IsNotExist(err) {
				t.Fatal("old path remains")
			}
			if got := testGit(t, repo, "diff-tree", "--no-commit-id", "--name-only", "-r", sha); !strings.Contains(got, old) || !strings.Contains(got, d.Rel) {
				t.Fatal(got)
			}
		})
	}
}

func TestLocalCommitIncludesUpdatedReports(t *testing.T) {
	repo, d := commitFixture(t)
	other := "reports/2026/08/other-report.md"
	os.MkdirAll(filepath.Dir(filepath.Join(repo, other)), 0755)
	os.WriteFile(filepath.Join(repo, other), []byte("original other report"), 0644)
	testGit(t, repo, "add", other)
	testGit(t, repo, "commit", "-m", "Other report")
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	testGit(t, repo, "clone", "--local", repo, snapshot)
	content, _ := os.ReadFile(filepath.Join(d.Dir, d.Rel))
	d.Dir = snapshot
	os.MkdirAll(filepath.Dir(filepath.Join(snapshot, d.Rel)), 0755)
	os.WriteFile(filepath.Join(snapshot, d.Rel), content, 0644)
	os.WriteFile(filepath.Join(snapshot, other), []byte("---\ncode: ABC123\n---\nother report with a new source"), 0644)
	d.Updates = []fileJob{{Rel: other, Slug: "other-report", Title: "Other report"}}
	sha, err := commitReport(context.Background(), repo, "case-update", d)
	if err != nil {
		t.Fatal(err)
	}
	if got := testGit(t, repo, "diff-tree", "--no-commit-id", "--name-only", "-r", sha); !strings.Contains(got, other) || !strings.Contains(got, d.Rel) {
		t.Fatal(got)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, other)); string(b) != "---\ncode: ABC123\n---\nother report with a new source" {
		t.Fatal(string(b))
	}
	if msg := testGit(t, repo, "log", "-1", "--format=%s"); msg != "Add report: Fixture report (and update other-report)" {
		t.Fatal(msg)
	}
	d.Updates[0].Rel = "../outside.md"
	if _, err = commitReport(context.Background(), repo, "case-bad", d); err == nil {
		t.Fatal("invalid update path accepted")
	}
}

func TestLocalCommitRefusesMissingOrConcurrentlyClaimedCode(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "concurrent claim", true: "missing"}[missing], func(t *testing.T) {
			repo, d := commitFixture(t)
			if missing {
				file := filepath.Join(d.Dir, d.Rel)
				b, _ := os.ReadFile(file)
				os.WriteFile(file, []byte(strings.Replace(string(b), "code: 7KQ2MX\n", "", 1)), 0644)
			} else {
				other := "reports/2026/10/another-report.md"
				os.MkdirAll(filepath.Dir(filepath.Join(repo, other)), 0755)
				os.WriteFile(filepath.Join(repo, other), []byte("---\ncode: 7KQ2MX\n---\nAnother report.\n"), 0644)
				testGit(t, repo, "add", other)
				testGit(t, repo, "commit", "-m", "Concurrent report")
			}
			before := testGit(t, repo, "rev-parse", "HEAD")
			if _, err := commitReport(context.Background(), repo, "case-code", d); err == nil || !strings.Contains(err.Error(), "code") {
				t.Fatal("invalid publication accepted", err)
			}
			if after := testGit(t, repo, "rev-parse", "HEAD"); after != before {
				t.Fatal("committed invalid code")
			}
			if _, err := os.Stat(filepath.Join(repo, d.Rel)); !os.IsNotExist(err) {
				t.Fatal("wrote rejected report", err)
			}
		})
	}
}
