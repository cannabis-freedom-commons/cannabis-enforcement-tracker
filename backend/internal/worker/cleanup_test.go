package worker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// A finished request's workspace keeps its logs and evidence; the build leftovers and the
// repository copy go.
func TestPruneKeepsLogsOnly(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"research-events.jsonl.zst", "research-stderr.log", "evidence-1-abc.json.zst", "package.json", "node_modules/x/index.js", "dist/index.html", "reports/2026/10/a.md", ".research-input/source.txt"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	research.PruneBuild(dir)
	for _, gone := range []string{"node_modules", "dist"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Fatal(gone, "kept after PruneBuild")
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "reports/2026/10/a.md")); err != nil {
		t.Fatal("PruneBuild removed the draft")
	}
	first, err := pruneIfNeeded(dir)
	second, err2 := pruneIfNeeded(dir)
	if !first || second || err != nil || err2 != nil {
		t.Fatal("prune should run once")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Fatalf("kept %d entries", len(entries))
	}
}

func cleanupFixture(t *testing.T, status string) (*Worker, string) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	id, err := s.Accept(context.Background(), store.Intake{Origin: "web", URL: "https://news.example/cleanup", CanonicalURL: "https://news.example/cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("UPDATE cases SET status=? WHERE id=?", status, v.CaseID); err != nil {
		t.Fatal(err)
	}
	return &Worker{Store: s, Config: config.Config{Workspaces: t.TempDir(), JobLogRetention: 14 * 24 * time.Hour}}, v.CaseID
}

func jobDir(t *testing.T, w *Worker, cid, suffix string, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(w.Config.Workspaces, cid+"-"+suffix)
	for _, name := range []string{".git/objects/pack", "node_modules/module/index.js", "reports/draft.md", "research-events.jsonl", "evidence-1-abc.json"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("evidence"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-age)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCompletionCleansImmediatelyButKeepsLogsAndCheckpoint(t *testing.T) {
	for _, status := range []string{"committed", "failed", "queued", "insufficient_evidence"} {
		t.Run(status, func(t *testing.T) {
			w, cid := cleanupFixture(t, status)
			dir := jobDir(t, w, cid, "123", 0)
			if _, err := w.Store.DB.Exec("INSERT INTO research_checkpoints VALUES(?,?,?)", cid, `{"saved":"research"}`, time.Now().Unix()); err != nil {
				t.Fatal(err)
			}
			if err := w.cleanWorkspaces(context.Background(), cid); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{".git", "node_modules", "reports"} {
				if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
					t.Fatal("left checkout/cache", name, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "research-events.jsonl")); err != nil {
				t.Fatal("lost log", err)
			}
			var n int
			if err := w.Store.DB.QueryRow("SELECT count(*) FROM research_checkpoints WHERE case_id=?", cid).Scan(&n); err != nil || n != 1 {
				t.Fatal("lost retry checkpoint", n, err)
			}
		})
	}
}

func TestSweepProtectsRunningJobsPendingPublicationSourcesAndSymlinks(t *testing.T) {
	w, cid := cleanupFixture(t, "publishing")
	dir := jobDir(t, w, cid, "123", time.Hour)
	if _, err := w.Store.DB.Exec("INSERT INTO jobs(id,kind,ref,payload,available_at) VALUES(?,'publish',?,?,0)", store.ID(), cid, store.JSON(draftJob{Dir: dir})); err != nil {
		t.Fatal(err)
	}
	old := jobDir(t, w, cid, "456", time.Hour)
	sources := filepath.Join(w.Config.Workspaces, "sources", cid, "capture")
	if err := os.MkdirAll(sources, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sources, "page.txt"), []byte("source evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "keep"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(w.Config.Workspaces, store.ID()+"-999")); err != nil {
		t.Fatal(err)
	}
	if err := w.CleanWorkspaces(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(dir, "reports/draft.md"), filepath.Join(sources, "page.txt"), filepath.Join(external, "keep")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("removed protected data", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(old, ".git")); !os.IsNotExist(err) {
		t.Fatal("kept superseded checkout", err)
	}
	// Even an expired lease may still have a process shutting down; never touch it.
	if _, err := w.Store.DB.Exec("UPDATE jobs SET status='running',lease_until=0 WHERE ref=? AND kind='research'", cid); err != nil {
		t.Fatal(err)
	}
	live := jobDir(t, w, cid, "789", time.Hour)
	if err := w.CleanWorkspaces(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(live, "node_modules")); err != nil {
		t.Fatal("removed active workspace", err)
	}
}

func TestRetentionOnlyRemovesExpiredFinishedJobs(t *testing.T) {
	for _, status := range []string{"failed", "committed", "queued"} {
		t.Run(status, func(t *testing.T) {
			w, cid := cleanupFixture(t, status)
			old := jobDir(t, w, cid, "123", 15*24*time.Hour)
			recent := jobDir(t, w, cid, "456", time.Hour)
			if _, err := w.Store.DB.Exec("UPDATE cases SET updated_at=? WHERE id=?", time.Now().Add(-15*24*time.Hour).Unix(), cid); err != nil {
				t.Fatal(err)
			}
			if err := w.CleanWorkspaces(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, err := os.Stat(old)
			if status == "queued" {
				if err != nil {
					t.Fatal("lost unfinished evidence", err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("kept expired logs", err)
			}
			if _, err := os.Stat(filepath.Join(recent, "research-events.jsonl")); err != nil {
				t.Fatal("lost recent logs", err)
			}
		})
	}
}

func TestSweepLeavesRecentUnknownDirectoriesAndRemovesOldOrphans(t *testing.T) {
	w, _ := cleanupFixture(t, "queued")
	recent := jobDir(t, w, store.ID(), "123", 0)
	old := jobDir(t, w, store.ID(), "456", time.Hour)
	if err := w.CleanWorkspaces(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatal("removed recent directory", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("kept orphan", err)
	}
}

func TestScreenshotArchiveWaitsForTerminalCaseAndIdleJobs(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node unavailable")
	}
	for _, status := range []string{"queued", "researching", "publishing", "committed", "failed"} {
		t.Run(status, func(t *testing.T) {
			w, cid := cleanupFixture(t, status)
			w.Config.RepoPath = t.TempDir()
			scripts := filepath.Join(w.Config.RepoPath, "scripts")
			os.MkdirAll(scripts, 0700)
			os.WriteFile(filepath.Join(scripts, "archive-screenshots.mjs"), []byte(`import fs from 'node:fs'; fs.writeFileSync(process.argv[2]+'/invoked','yes');`), 0600)
			dir := filepath.Join(w.Config.Workspaces, "sources", cid)
			os.MkdirAll(dir, 0700)
			if err := w.archiveScreenshots(context.Background(), cid); err != nil {
				t.Fatal(err)
			}
			_, err := os.Stat(filepath.Join(dir, "invoked"))
			if (err == nil) != store.Terminal(status) {
				t.Fatal("unexpected archive invocation", status, err)
			}
			os.Remove(filepath.Join(dir, "invoked"))
			w.Store.DB.Exec("UPDATE jobs SET status='running' WHERE ref=?", cid)
			if err := w.archiveScreenshots(context.Background(), cid); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "invoked")); !os.IsNotExist(err) {
				t.Fatal("archived while job running")
			}
		})
	}
}
