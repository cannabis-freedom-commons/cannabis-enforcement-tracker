package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// batchFixture: an origin, the backend's clone of it on the publish branch, and another clone
// that pushes to main as people do.
func batchFixture(t *testing.T) (origin, server, other string) {
	t.Helper()
	seed, _ := commitFixture(t)
	testGit(t, seed, "branch", "-M", "main")
	origin = filepath.Join(t.TempDir(), "origin.git")
	testGit(t, seed, "clone", "--quiet", "--bare", seed, origin)
	server = filepath.Join(t.TempDir(), "server")
	testGit(t, seed, "clone", "--quiet", origin, server)
	other = filepath.Join(t.TempDir(), "other")
	testGit(t, seed, "clone", "--quiet", origin, other)
	for _, repo := range []string{server, other} {
		testGit(t, repo, "config", "user.name", "Fixture")
		testGit(t, repo, "config", "user.email", "fixture@localhost")
	}
	if err := ensurePublishBranch(context.Background(), server, "submissions"); err != nil {
		t.Fatal(err)
	}
	return origin, server, other
}

// queueReport takes a submission through publication: its report is committed on the branch.
func queueReport(t *testing.T, w *Worker, n int) string {
	t.Helper()
	ctx := context.Background()
	u := "https://news.example/report-" + string(rune('a'+n))
	id, err := w.Store.Accept(ctx, store.Intake{Origin: "web", URL: u, CanonicalURL: u})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := w.Store.Get(ctx, id)
	research, _ := w.Store.ClaimKind(ctx, "jobs", "research", time.Minute)
	w.Store.Finish(ctx, "jobs", research, nil)
	slug := "batch-report-" + string(rune('a'+n))
	rel := "reports/2026/10/" + slug + ".md"
	dir := filepath.Join(w.Config.Workspaces, v.CaseID+"-1")
	os.MkdirAll(filepath.Join(dir, "reports/2026/10"), 0755)
	content := "---\ncode: " + string(rune('A'+n)) + "KQ2MX\n---\nCited report " + slug + ".\n"
	os.WriteFile(filepath.Join(dir, rel), []byte(content), 0644)
	sum := sha256.Sum256([]byte(content))
	d := draftJob{Dir: dir, Rel: rel, Slug: slug, Title: "Report " + slug, Digest: hex.EncodeToString(sum[:])}
	tx, _ := w.Store.DB.BeginTx(ctx, nil)
	store.Enqueue(tx, "jobs", "publish", v.CaseID, store.JSON(d), time.Now().Unix())
	tx.Commit()
	job, err := w.Store.ClaimKind(ctx, "jobs", "publish", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.publish(ctx, job); err != nil {
		t.Fatal(err)
	}
	if v, _ = w.Store.Get(ctx, id); v.Status != "committed" || !strings.Contains(v.Explanation, "next batch") {
		t.Fatal("not committed for the next batch", v.Status, v.Explanation)
	}
	return id
}

// Reports wait on the publish branch, go to main together in one fast-forward on top of what
// others pushed (no merge commit), and are published once the live site contains them.
func TestReportsGoToMainInBatchesAndArePublishedWhenLive(t *testing.T) {
	origin, server, other := batchFixture(t)
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "store.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	deployed := ""
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"commit":"` + deployed + `"}`))
	}))
	defer site.Close()
	w := &Worker{Store: s, Config: config.Config{RepoPath: server, Workspaces: t.TempDir(), AutoPublish: true, AutoPush: true, RepoSync: true,
		PublishBranch: "submissions", PublishInterval: 30 * time.Minute, PublishOffset: 5 * time.Minute, PublicURL: site.URL}}
	mainBefore := testGit(t, origin, "rev-parse", "main")
	first := queueReport(t, w, 0)
	// someone pushes to main while the reports wait
	os.WriteFile(filepath.Join(other, "elsewhere.txt"), []byte("someone else's work"), 0644)
	testGit(t, other, "add", ".")
	testGit(t, other, "commit", "-m", "Pushed elsewhere")
	testGit(t, other, "push", "--quiet", "origin", "HEAD:main")
	second := queueReport(t, w, 1)
	if testGit(t, origin, "rev-parse", "main") == mainBefore {
		t.Fatal("test setup: the other push is missing")
	}
	if strings.Contains(testGit(t, origin, "log", "--format=%s", "main"), "batch-report") {
		t.Fatal("a report reached main before its batch")
	}
	if !strings.Contains(testGit(t, origin, "log", "--format=%B", "submissions"), "Submission-Case") {
		t.Fatal("the publish branch was not pushed")
	}

	if err := w.mergeBatch(ctx); err != nil {
		t.Fatal(err)
	}
	head := testGit(t, origin, "rev-parse", "main")
	if testGit(t, origin, "rev-parse", "submissions") != head {
		t.Fatal("the publish branch does not equal main after the batch")
	}
	if merges := testGit(t, origin, "rev-list", "--merges", "main"); merges != "" {
		t.Fatal("the batch made a merge commit", merges)
	}
	if testGit(t, origin, "show", "main:elsewhere.txt") != "someone else's work" {
		t.Fatal("others' work lost")
	}
	for _, id := range []string{first, second} {
		if v, _ := s.Get(ctx, id); v.Status != "deploying" {
			t.Fatal("case not deploying after the batch", v.Status, v.Explanation)
		}
	}
	var reports int
	s.DB.QueryRow("SELECT reports FROM publication_batches WHERE commit_sha=?", head).Scan(&reports)
	if reports != 2 {
		t.Fatal("batch not recorded", reports)
	}
	// nothing new: no push, no batch
	if err := w.mergeBatch(ctx); err != nil || testGit(t, origin, "rev-parse", "main") != head {
		t.Fatal("an empty batch changed main", err)
	}

	// the Pages build on GitHub: the page links it, and says whether it builds this batch
	runSHA := mainBefore
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/actions/workflows/deploy.yml/runs") || r.URL.Query().Get("branch") != "main" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"workflow_runs":[{"html_url":"https://github.com/owner/repo/actions/runs/42","status":"in_progress","conclusion":null,"head_sha":"` + runSHA + `","run_started_at":"2026-11-09T05:13:00Z","updated_at":"2026-11-09T05:14:00Z"}]}`))
	}))
	defer github.Close()
	githubAPI, w.Config.GitHubRepo = github.URL, "owner/repo"
	defer func() { githubAPI = "https://api.github.com" }()
	lastRunCheck = time.Time{}
	w.refreshDeployRun(ctx)
	if run := s.DeployRun(); run == nil || run.URL != "https://github.com/owner/repo/actions/runs/42" || run.Status != "in_progress" || run.IncludesBatch {
		t.Fatal("deploy run of an older commit", run)
	}
	runSHA, lastRunCheck = head, time.Time{}
	w.refreshDeployRun(ctx)
	if run := s.DeployRun(); run == nil || !run.IncludesBatch {
		t.Fatal("deploy run of the batch not recognised", run)
	}

	// the site still shows the build from before the batch
	deployed = mainBefore
	if err := w.watchDeploys(ctx); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Get(ctx, first); v.Status != "deploying" {
		t.Fatal("published before the site had it", v.Status)
	}
	deployed = head
	if err := w.watchDeploys(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first, second} {
		if v, _ := s.Get(ctx, id); v.Status != "published" || !strings.Contains(v.Explanation, "/report/batch-report-") {
			t.Fatal("case not published once live", v.Status, v.Explanation)
		}
	}
	var deployedAt int64
	s.DB.QueryRow("SELECT deployed_at FROM publication_batches").Scan(&deployedAt)
	if deployedAt == 0 {
		t.Fatal("batch not marked deployed")
	}
}

func TestNextBatchIsOnTheClock(t *testing.T) {
	c := config.Config{PublishInterval: 30 * time.Minute, PublishOffset: 5 * time.Minute}
	at := func(s string) time.Time { v, _ := time.Parse("15:04:05", s); return v }
	for now, want := range map[string]string{"10:04:59": "10:05:00", "10:05:00": "10:35:00", "10:20:00": "10:35:00", "10:35:00": "11:05:00", "10:59:00": "11:05:00"} {
		if got := c.NextBatch(at(now)).Format("15:04:05"); got != want {
			t.Errorf("at %s: next batch %s, want %s", now, got, want)
		}
	}
}

// In the backend's clone a plain git push or pull touches only the publish branch, and a manual
// push to main is refused. Reports that reach main some other way anyway are taken as on main:
// the next batch pushes nothing again, and they are published once the site has them.
func TestReportsPushedToMainByHandAreRecovered(t *testing.T) {
	origin, server, other := batchFixture(t)
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "store.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	deployed := ""
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"commit":"` + deployed + `"}`))
	}))
	defer site.Close()
	w := &Worker{Store: s, Config: config.Config{RepoPath: server, Workspaces: t.TempDir(), AutoPublish: true, AutoPush: true, RepoSync: true,
		PublishBranch: "submissions", PublishInterval: 30 * time.Minute, PublishOffset: 5 * time.Minute, PublicURL: site.URL}}
	if got := testGit(t, server, "config", "branch.submissions.merge"); got != "refs/heads/submissions" {
		t.Fatal("the publish branch tracks", got)
	}
	mainBefore := testGit(t, origin, "rev-parse", "main")
	id := queueReport(t, w, 0)

	// a plain push from the clone goes to the publish branch, a push to main is refused
	testGit(t, server, "push", "--quiet")
	if testGit(t, origin, "rev-parse", "main") != mainBefore {
		t.Fatal("a plain push from the clone reached main")
	}
	cmd := exec.Command("git", "push", "--quiet", "origin", "HEAD:main")
	cmd.Dir = server
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "only the backend pushes to main") {
		t.Fatal("a manual push to main was not refused", err, string(out))
	}

	// it happens anyway, from elsewhere
	testGit(t, other, "fetch", "--quiet", "origin")
	testGit(t, other, "push", "--quiet", "origin", "origin/submissions:refs/heads/main")
	head := testGit(t, origin, "rev-parse", "main")
	if head == mainBefore {
		t.Fatal("test setup: the reports did not reach main")
	}
	if err := w.mergeBatch(ctx); err != nil {
		t.Fatal(err)
	}
	var batches int
	s.DB.QueryRow("SELECT count(*) FROM publication_batches").Scan(&batches)
	if testGit(t, origin, "rev-parse", "main") != head || batches != 0 {
		t.Fatal("the reports already on main were pushed again", batches)
	}
	deployed = mainBefore
	if err := w.watchDeploys(ctx); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Get(ctx, id); v.Status != "deploying" {
		t.Fatal("a report on main is not deploying", v.Status)
	}
	deployed = head
	if err := w.watchDeploys(ctx); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Get(ctx, id); v.Status != "published" {
		t.Fatal("a report on the site is not published", v.Status)
	}

	// someone checks out main in the clone: the backend goes back to the publish branch
	testGit(t, server, "checkout", "--quiet", "main")
	if err := ensurePublishBranch(ctx, server, "submissions"); err != nil {
		t.Fatal(err)
	}
	if got := testGit(t, server, "rev-parse", "--abbrev-ref", "HEAD"); got != "submissions" {
		t.Fatal("the clone stays on", got)
	}
}
