package worker

import (
	"context"
	"errors"
	"net/http/httptest"
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

const testWorkerToken = "test-worker-token-0123456789abcdef0123456789"

// remoteBackend starts the backend's worker API over s and returns a client of it.
func remoteBackend(t *testing.T, s *store.Store, c config.Config) (*Client, *API) {
	t.Helper()
	c.ResearchMode = "remote"
	c.WorkerTokens = map[string]string{testWorkerToken: "home"}
	if c.Workspaces == "" {
		c.Workspaces = t.TempDir()
	}
	if c.RunCents == 0 {
		c.RunCents = 1
	}
	api := &API{Worker: &Worker{Store: s, Config: c}, Health: research.NewHealth()}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)
	return &Client{URL: server.URL, Token: testWorkerToken}, api
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return s
}

// A worker elsewhere does the whole research job through the API: runs, transcript, checkpoint
// and the outcome land in the backend's database, the run's logs in the backend's job storage.
func TestRemoteWorkerResearchesThroughTheAPI(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS local runner")
	}
	repo, _ := commitFixture(t)
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
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	original := "---\ncode: ABC123\ncountry: it\ndate: 2026-11-03\ntitle: Police arrest two men\ncreated_by_model: original/model\nstatus: reported\nsources:\n  - url: https://first.example/article\n    title: First account\n    publisher: First\n---\n\nPolice arrested two men at the farm. [@1]\n"
	path := filepath.Join(repo, "reports/2026/11/existing-case.md")
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "add", ".")
	testGit(t, repo, "commit", "-m", "Fixture inputs")
	s := openStore(t)
	ctx := context.Background()
	url := "https://second.example/article"
	id, err := s.Accept(ctx, store.Intake{Origin: "web", URL: url, CanonicalURL: url})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := s.Get(ctx, id)
	sources := []research.Source{{URL: url, Title: "Second account", Publisher: "Second"}}
	good := research.Result{Outcome: "duplicate", ExistingReport: "existing-case", Explanation: "The original report already covers the farm arrests and the supported facts.", Sources: sources}
	initial := safeurl.Evidence{URL: url, FinalURL: url, Content: "Police arrested two men at the farm."}
	// A saved coverage review skips the source crawl, so the fixture needs no network.
	saved := researchCheckpoint{Result: good, Initial: initial, Evidence: []safeurl.Evidence{initial}, Model: "fixture/model", Dir: "missing-old-checkout", ReviewCoverage: true}
	if _, err := s.DB.Exec("INSERT INTO research_checkpoints VALUES(?,?,?)", v.CaseID, store.JSON(saved), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	answer := store.JSON(map[string]any{"type": "text", "part": map[string]string{"text": store.JSON(good)}})
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 1.0.0; exit 0; fi\necho '{\"type\":\"step_finish\",\"part\":{\"tokens\":{\"input\":7,\"output\":3}}}'\ncat <<'RESPONSE'\n" + answer + "\nRESPONSE\n"
	binary := filepath.Join(bin, "opencode")
	os.WriteFile(binary, []byte(script), 0755)
	providerConfig := filepath.Join(bin, "opencode.json")
	os.WriteFile(providerConfig, []byte(`{}`), 0600)

	backendWorkspaces := t.TempDir()
	client, _ := remoteBackend(t, s, config.Config{RepoPath: t.TempDir(), Workspaces: backendWorkspaces})
	job, ok, err := client.Claim(ctx)
	if err != nil || !ok || job.Ref != v.CaseID {
		t.Fatal("no job leased", ok, err)
	}
	// the worker has no database: a nil Store would panic on any direct use
	w := Worker{Backend: client.Job(job), Config: config.Config{RepoPath: repo, Workspaces: t.TempDir(), ResearchRunner: "local", OpenCodeBinary: binary, OpenCodeConfigFile: providerConfig, OpenCodeAuthFile: filepath.Join(bin, "missing-auth.json"), ResearchModel: "fixture/model", RunCents: 1}}
	if err := w.process(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(ctx, job, nil); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Get(ctx, id)
	if v.Status != "duplicate" || v.ReportSlug != "existing-case" {
		t.Fatal("outcome not recorded", v.Status, v.ReportSlug)
	}
	var workspace, model string
	var input int64
	if err := s.DB.QueryRow("SELECT workspace,model,input_tokens FROM research_runs WHERE case_id=? AND status='completed'", v.CaseID).Scan(&workspace, &model, &input); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(workspace, backendWorkspaces) || model != "fixture/model" || input != 7 {
		t.Fatal("run not recorded on the backend", workspace, model, input)
	}
	if _, err := os.Stat(filepath.Join(workspace, "research-events.jsonl")); err != nil {
		t.Fatal("run log not uploaded", err)
	}
	var events int
	s.DB.QueryRow("SELECT count(*) FROM ai_events WHERE case_id=?", v.CaseID).Scan(&events)
	var jobStatus string
	s.DB.QueryRow("SELECT status FROM jobs WHERE id=?", job.ID).Scan(&jobStatus)
	if events == 0 || jobStatus != "done" {
		t.Fatal("transcript or job completion missing", events, jobStatus)
	}
}

// Every call needs the job's current lease, may touch only that job's case, and cannot publish.
func TestWorkerAPIRefusesCallsOutsideTheLease(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	first, _ := s.Accept(ctx, store.Intake{Origin: "web", URL: "https://a.example/1", CanonicalURL: "https://a.example/1"})
	second, _ := s.Accept(ctx, store.Intake{Origin: "web", URL: "https://b.example/2", CanonicalURL: "https://b.example/2"})
	client, _ := remoteBackend(t, s, config.Config{})
	if err := (&Client{URL: client.URL, Token: "wrong"}).Hello(ctx); err == nil {
		t.Fatal("unknown token accepted")
	}
	job, ok, err := client.Claim(ctx)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	a, _ := s.Get(ctx, first)
	b, _ := s.Get(ctx, second)
	mine, other := a.CaseID, b.CaseID
	if job.Ref != mine {
		mine, other = other, mine
	}
	db := client.Job(job)
	if err := db.Transition(ctx, mine, "fetching", "Opening the submitted source.", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Transition(ctx, other, "fetching", "Opening the submitted source.", "", ""); err == nil {
		t.Fatal("changed another job's case")
	}
	if err := db.Transition(ctx, mine, "committed", "Committed.", "", ""); err == nil {
		t.Fatal("a worker published")
	}
	stale := job
	stale.Token = "not-the-lease"
	if err := client.Job(stale).Transition(ctx, mine, "classifying", "Classifying.", "", ""); !errors.Is(err, errLeaseLost) {
		t.Fatal("call without the lease was not refused as lost", err)
	}
	if _, err := db.Checkpoint(ctx, mine); err != nil {
		t.Fatal("no checkpoint should read as empty", err)
	}
	if _, err := db.Reserve(ctx, mine, 0, 0); err != nil {
		t.Fatal("budget is the backend's, not the worker's", err)
	}
	// a page capture is uploaded to the backend, which serves it on the request page
	capture := filepath.Join(t.TempDir(), "capture")
	os.MkdirAll(capture, 0700)
	os.WriteFile(filepath.Join(capture, "screenshot.png"), []byte("png"), 0600)
	captureID := store.ID()
	if err := db.RecordCapture(ctx, mine, safeurl.Evidence{CaptureID: captureID, URL: "https://a.example/1", ArtifactDir: capture}); err != nil {
		t.Fatal(err)
	}
	var dir string
	if err := s.DB.QueryRow("SELECT artifact_dir FROM source_captures WHERE id=?", captureID).Scan(&dir); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "screenshot.png")); err != nil || string(b) != "png" || dir == capture {
		t.Fatal("capture not stored on the backend", dir, err)
	}
	if err := client.Finish(ctx, job, store.ErrInterrupted); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var status string
	s.DB.QueryRow("SELECT status,attempts FROM jobs WHERE id=?", job.ID).Scan(&status, &attempts)
	if status != "queued" || attempts != 0 {
		t.Fatal("an interrupted remote attempt was counted", status, attempts)
	}
}

// A draft researched on a worker is uploaded with the blobs it started from and committed by
// the backend; a report changed on the backend meanwhile sends it back for review instead.
func TestRemoteDraftIsCommittedByTheBackend(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "changed meanwhile"}[changed], func(t *testing.T) {
			repo, d := commitFixture(t)
			old := "reports/2026/09/fixture-report.md"
			os.MkdirAll(filepath.Dir(filepath.Join(repo, old)), 0755)
			os.WriteFile(filepath.Join(repo, old), []byte("original report"), 0644)
			testGit(t, repo, "add", old)
			testGit(t, repo, "commit", "-m", "Original report")
			s := openStore(t)
			ctx := context.Background()
			id, _ := s.Accept(ctx, store.Intake{Origin: "web", URL: "https://news.example/a", CanonicalURL: "https://news.example/a"})
			v, _ := s.Get(ctx, id)
			// the worker's clone and its job workspace, named as research names them
			clone := filepath.Join(t.TempDir(), "clone")
			testGit(t, repo, "clone", "--quiet", repo, clone)
			workspace := filepath.Join(t.TempDir(), v.CaseID+"-1")
			testGit(t, clone, "clone", "--quiet", "--local", clone, workspace)
			content, _ := os.ReadFile(filepath.Join(d.Dir, d.Rel))
			os.MkdirAll(filepath.Dir(filepath.Join(workspace, d.Rel)), 0755)
			os.WriteFile(filepath.Join(workspace, d.Rel), content, 0644)
			os.Remove(filepath.Join(workspace, old))
			d.Dir, d.PreviousRel = workspace, old
			if changed {
				os.WriteFile(filepath.Join(repo, old), []byte("newer report"), 0644)
				testGit(t, repo, "add", old)
				testGit(t, repo, "commit", "-m", "Concurrent update")
			}
			backendWorkspaces := t.TempDir()
			client, api := remoteBackend(t, s, config.Config{RepoPath: repo, Workspaces: backendWorkspaces, AutoPublish: true})
			client.Repo = clone
			job, ok, err := client.Claim(ctx)
			if err != nil || !ok {
				t.Fatal(ok, err)
			}
			if _, err := s.DB.Exec("INSERT INTO research_checkpoints VALUES(?,?,?)", v.CaseID, store.JSON(researchCheckpoint{}), time.Now().Unix()); err != nil {
				t.Fatal(err)
			}
			if err := client.Job(job).QueuePublish(ctx, job, d); err != nil {
				t.Fatal(err)
			}
			if err := client.Finish(ctx, job, nil); err != nil {
				t.Fatal(err)
			}
			publish, err := s.ClaimKind(ctx, "jobs", "publish", time.Minute)
			if err != nil {
				t.Fatal("publish job not queued", err)
			}
			if !strings.Contains(publish.Payload, backendWorkspaces) {
				t.Fatal("draft not kept in the backend's job storage", publish.Payload)
			}
			err = api.Worker.publish(ctx, publish)
			v, _ = s.Get(ctx, id)
			if changed {
				if !errors.Is(err, errCoverageRequeued) || v.Status != "queued" {
					t.Fatal("stale draft was not sent back for review", err, v.Status)
				}
				return
			}
			if err != nil || v.Status != "committed" {
				t.Fatal("draft not committed", err, v.Status)
			}
			if got := testGit(t, repo, "show", "HEAD:"+d.Rel); got != strings.TrimSpace(string(content)) {
				t.Fatal("committed draft differs", got)
			}
			if _, err := os.Stat(filepath.Join(repo, old)); !os.IsNotExist(err) {
				t.Fatal("old path remains")
			}
		})
	}
}

// Restarting the backend leaves research leased to remote workers running; a local backend's
// own research is put back in the queue as before.
func TestBackendRestartKeepsRemoteLeases(t *testing.T) {
	for _, remote := range []bool{false, true} {
		s := openStore(t)
		ctx := context.Background()
		s.Accept(ctx, store.Intake{Origin: "web", URL: "https://news.example/a", CanonicalURL: "https://news.example/a"})
		job, err := s.ClaimKind(ctx, "jobs", "research", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.RecoverKeeping(ctx, remote); err != nil {
			t.Fatal(err)
		}
		if err := s.Heartbeat(ctx, "jobs", job, time.Minute); (err == nil) != remote {
			t.Fatal("remote", remote, "lease after restart:", err)
		}
	}
}
