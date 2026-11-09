package worker

// Publication in batches (PUBLISH_BRANCH): each validated report is committed to the publish branch
// and pushed there, so workers see it at once. Every PUBLISH_INTERVAL, at PUBLISH_OFFSET on the
// clock (30m and 5m: :05 and :35, just before the site is built at :13 and :43), the branch,
// rebased onto main, is pushed to main in one fast-forward, so
// main gets the reports' own commits and no merge commit; with nothing new, nothing is pushed. The
// reports are then deploying, and published once the live site's /version.json names a commit
// that contains them (the site is rebuilt on its own schedule).
//
// Without a publish branch, each report is pushed to main at once and the deploy watch still
// marks it published when it is live.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

const mainBranch = "main"

func gitOutput(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// backendPush is set in the environment of the backend's own git commands; the pre-push hook it
// installs lets only those push to main from its clone.
const backendPush = "CET_BACKEND_PUSH"

// prePushHook refuses a person's push to main from the backend's clone: reports reach main only
// in the backend's batches. The backend's pushes carry backendPush.
const prePushHook = `#!/bin/sh
# Installed by the tracker backend (backend/internal/worker/batch.go); it is rewritten at start.
# In this clone, reports wait on the publish branch and the backend pushes them to main in
# batches. A person's push to main from here would skip that, so it is refused.
[ -n "$` + backendPush + `" ] && exit 0
while read -r local_ref local_sha remote_ref remote_sha; do
  if [ "$remote_ref" = "refs/heads/` + mainBranch + `" ]; then
    echo "Refused: in the backend's clone only the backend pushes to main (docs/DEPLOY.md, Publication)." >&2
    exit 1
  fi
done
exit 0
`

// installPrePushHook writes prePushHook into the clone, unless someone else's hook is there.
func installPrePushHook(ctx context.Context, repo string) error {
	path, err := gitOutput(ctx, repo, "rev-parse", "--git-path", "hooks/pre-push")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(repo, path)
	}
	if old, err := os.ReadFile(path); err == nil && !strings.Contains(string(old), "Installed by the tracker backend") {
		slog.Warn("the clone has its own pre-push hook; manual pushes to main are not guarded", "hook", path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(prePushHook), 0755)
}

// ensurePublishBranch checks out the publish branch (made from main the first time). Its upstream
// is itself on origin, so a plain git push or pull here touches only the branch; the backend
// rebases onto main by name (pullRepo). A hook refuses manual pushes to main.
func ensurePublishBranch(ctx context.Context, repo, branch string) error {
	unlock, err := publishLock(ctx, repo)
	if err != nil {
		return err
	}
	defer unlock()
	if err := gitQuiet(ctx, repo, 2*time.Minute, "fetch", "--quiet", "origin"); err != nil {
		return err
	}
	current, err := gitOutput(ctx, repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	if current != branch {
		args := []string{"checkout", "--quiet", "-b", branch, "origin/" + mainBranch}
		if _, e := gitOutput(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); e == nil {
			args = []string{"checkout", "--quiet", branch}
		} else if _, e := gitOutput(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch); e == nil {
			args = []string{"checkout", "--quiet", "-b", branch, "origin/" + branch}
		}
		if err := gitQuiet(ctx, repo, time.Minute, args...); err != nil {
			return fmt.Errorf("checking out %s: %w", branch, err)
		}
	}
	for key, value := range map[string]string{"branch." + branch + ".remote": "origin", "branch." + branch + ".merge": "refs/heads/" + branch} {
		if err := gitQuiet(ctx, repo, time.Minute, "config", key, value); err != nil {
			return err
		}
	}
	if err := installPrePushHook(ctx, repo); err != nil {
		slog.Warn("the pre-push hook could not be installed", "error", err)
	}
	// a person's `git pull` here rebases as well, instead of merging main into the branch
	return gitQuiet(ctx, repo, time.Minute, "config", "pull.rebase", "true")
}

// pushBranch pushes the publish branch; it is rebased onto main before each commit, so it is
// forced. Only the backend writes it.
func pushBranch(ctx context.Context, repo, branch string) error {
	pushMu.Lock()
	defer pushMu.Unlock()
	return gitQuiet(ctx, repo, 90*time.Second, "push", "--quiet", "--force", "origin", "HEAD:refs/heads/"+branch)
}

var caseTrailer = regexp.MustCompile(`(?m)^Submission-Case: ([0-9a-f]{32})\s*$`)

// caseCommits lists the commits in range (as git log takes it) with the case each commits.
func caseCommits(ctx context.Context, repo, revs string) (map[string]string, error) {
	out, err := gitOutput(ctx, repo, "log", "--format=%H%x1f%B%x1e", revs)
	if err != nil {
		return nil, err
	}
	cases := map[string]string{}
	for _, entry := range strings.Split(out, "\x1e") {
		sha, body, ok := strings.Cut(strings.TrimSpace(entry), "\x1f")
		if !ok {
			continue
		}
		for _, m := range caseTrailer.FindAllStringSubmatch(body, -1) {
			if _, seen := cases[m[1]]; !seen {
				cases[m[1]] = sha // the newest commit of the case
			}
		}
	}
	return cases, nil
}

// mergeBatch pushes the publish branch to main when it holds reports main does not have yet.
func (w *Worker) mergeBatch(ctx context.Context) error {
	repo, branch := w.Config.RepoPath, w.Config.PublishBranch
	unlock, err := publishLock(ctx, repo)
	if err != nil {
		return err
	}
	defer unlock()
	if err := pullRepo(ctx, repo, branch); err != nil {
		return fmt.Errorf("updating from main: %w", err)
	}
	ahead, err := gitOutput(ctx, repo, "rev-list", "--count", "origin/"+mainBranch+"..HEAD")
	if err != nil || ahead == "0" {
		return err
	}
	cases, err := caseCommits(ctx, repo, "origin/"+mainBranch+"..HEAD")
	if err != nil {
		return err
	}
	pushMu.Lock()
	err = gitQuiet(ctx, repo, 90*time.Second, "push", "--quiet", "origin", "HEAD:refs/heads/"+mainBranch)
	if err != nil {
		// someone pushed to main in between: once more on top of theirs
		if err = pullRepo(ctx, repo, branch); err == nil {
			if cases, err = caseCommits(ctx, repo, "origin/"+mainBranch+"..HEAD"); err == nil {
				err = gitQuiet(ctx, repo, 90*time.Second, "push", "--quiet", "origin", "HEAD:refs/heads/"+mainBranch)
			}
		}
	}
	if err == nil {
		// the branch now equals main
		if e := gitQuiet(ctx, repo, 90*time.Second, "push", "--quiet", "--force", "origin", "HEAD:refs/heads/"+branch); e != nil {
			slog.Warn("the publish branch could not be updated", "error", e)
		}
	}
	pushMu.Unlock()
	if err != nil {
		return fmt.Errorf("pushing the batch to main: %w", err)
	}
	head, err := gitOutput(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	res, err := w.Store.DB.ExecContext(ctx, "INSERT INTO publication_batches(merged_at,commit_sha,reports) VALUES(?,?,?)", time.Now().Unix(), head, len(cases))
	if err != nil {
		return err
	}
	batch, _ := res.LastInsertId()
	slog.Info("publication batch pushed to main", "commit", head, "reports", len(cases))
	others := ""
	if len(cases) > 1 {
		others = fmt.Sprintf(" with %d other reports", len(cases)-1)
	}
	for cid, sha := range cases {
		if _, err := w.Store.DB.ExecContext(ctx, "UPDATE local_publications SET commit_sha=?,batch_id=? WHERE case_id=?", sha, batch, cid); err != nil {
			return err
		}
		why := "The report went to main" + others + " (" + sha[:12] + "). It appears on the site with its next build (every 30 minutes, at :13 and :43)."
		if err := w.Store.Transition(ctx, cid, "deploying", why, "", ""); err != nil {
			slog.Warn("batch case not moved to deploying", "case", cid, "error", err)
		}
	}
	return nil
}

// deployedCommit is the commit the live site was built from (its /version.json).
func (w *Worker) deployedCommit(ctx context.Context) (string, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, w.Config.PublicURL+"/version.json?t="+fmt.Sprint(time.Now().Unix()/60), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version.json: %s", resp.Status)
	}
	var v struct {
		Commit string `json:"commit"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&v); err != nil {
		return "", err
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(v.Commit) {
		return "", errors.New("version.json names no commit")
	}
	return v.Commit, nil
}

func reachable(ctx context.Context, repo, rev string) (map[string]bool, error) {
	out, err := gitOutput(ctx, repo, "rev-list", rev)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, sha := range strings.Fields(out) {
		set[sha] = true
	}
	return set, nil
}

// watchDeploys moves committed reports that reached main to deploying, and deploying reports the
// live site contains to published.
func (w *Worker) watchDeploys(ctx context.Context) error {
	type pending struct{ cid, sha, status, slug string }
	rows, err := w.Store.DB.QueryContext(ctx, `SELECT c.id,coalesce(p.commit_sha,''),c.status,c.report_slug FROM cases c JOIN local_publications p ON p.case_id=c.id WHERE c.status IN ('committed','deploying')`)
	if err != nil {
		return err
	}
	var list []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.cid, &p.sha, &p.status, &p.slug); err != nil {
			rows.Close()
			return err
		}
		list = append(list, p)
	}
	rows.Close()
	if len(list) == 0 {
		return nil
	}
	w.refreshDeployRun(ctx)
	repo := w.Config.RepoPath
	deployed, err := w.deployedCommit(ctx)
	if err != nil {
		return err
	}
	if _, e := gitOutput(ctx, repo, "cat-file", "-e", deployed+"^{commit}"); e != nil {
		if err := gitQuiet(ctx, repo, 2*time.Minute, "fetch", "--quiet", "origin"); err != nil {
			return err
		}
	}
	live, err := reachable(ctx, repo, deployed)
	if err != nil {
		return err
	}
	onMain, err := reachable(ctx, repo, "origin/"+mainBranch)
	if err != nil {
		return err
	}
	var byCase map[string]string // read only when a recorded commit is not on main (rebased since)
	for _, p := range list {
		if !onMain[p.sha] {
			if byCase == nil {
				if byCase, err = caseCommits(ctx, repo, "origin/"+mainBranch); err != nil {
					return err
				}
			}
			sha, ok := byCase[p.cid]
			if !ok {
				continue // still waiting on the publish branch
			}
			p.sha = sha
			if _, err := w.Store.DB.ExecContext(ctx, "UPDATE local_publications SET commit_sha=? WHERE case_id=?", sha, p.cid); err != nil {
				return err
			}
		}
		if p.status == "committed" {
			if err := w.Store.Transition(ctx, p.cid, "deploying", "The report is on main ("+p.sha[:12]+"). It appears on the site with its next build (every 30 minutes, at :13 and :43).", "", ""); err != nil {
				slog.Warn("case not moved to deploying", "case", p.cid, "error", err)
				continue
			}
		}
		if live[p.sha] {
			why := "The report is on the site: " + w.Config.PublicURL + "/report/" + p.slug + "/"
			if err := w.Store.Transition(ctx, p.cid, "published", why, "", ""); err != nil {
				slog.Warn("case not marked published", "case", p.cid, "error", err)
			}
		}
	}
	_, err = w.Store.DB.ExecContext(ctx, `UPDATE publication_batches SET deployed_at=? WHERE deployed_at=0 AND NOT EXISTS(
 SELECT 1 FROM local_publications p JOIN cases c ON c.id=p.case_id WHERE p.batch_id=publication_batches.id AND c.status<>'published')`, time.Now().Unix())
	return err
}

// publicationLoop pushes batches on the clock and watches deploys, every minute until ctx ends.
func (w *Worker) publicationLoop(ctx context.Context) {
	if w.Config.PublishBranch != "" {
		if err := ensurePublishBranch(ctx, w.Config.RepoPath, w.Config.PublishBranch); err != nil {
			slog.Error("the publish branch could not be set up; reports wait", "branch", w.Config.PublishBranch, "error", err)
		}
	}
	next := w.Config.NextBatch(time.Now())
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		// someone switched the clone to another branch: back to the publish branch before a batch
		if b := w.Config.PublishBranch; b != "" {
			if current, err := gitOutput(ctx, w.Config.RepoPath, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && current != b {
				slog.Warn("the clone was on another branch; switching back to the publish branch", "branch", current)
				if err := ensurePublishBranch(ctx, w.Config.RepoPath, b); err != nil {
					slog.Error("the publish branch could not be checked out; reports wait", "error", err)
				}
			}
		}
		if w.Config.PublishBranch != "" && !time.Now().Before(next) {
			next = w.Config.NextBatch(time.Now())
			if err := w.mergeBatch(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("publication batch not pushed; trying again with the next one", "error", err)
			}
		}
		if err := w.watchDeploys(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("deploy check failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// deployWorkflow is the site's Pages workflow (.github/workflows/deploy.yml).
const deployWorkflow = "deploy.yml"

// githubAPI is GitHub's REST API; tests point it elsewhere.
var githubAPI = "https://api.github.com"

var lastRunCheck time.Time

// refreshDeployRun reads the Pages workflow's latest run on main from GitHub, at most every two
// minutes, for the submissions page's link to the build. Anonymous reads are limited to 60 an
// hour, so it is asked only while reports wait for the site (GITHUB_TOKEN raises the limit).
func (w *Worker) refreshDeployRun(ctx context.Context) {
	if time.Since(lastRunCheck) < 2*time.Minute {
		return
	}
	lastRunCheck = time.Now()
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	url := githubAPI + "/repos/" + w.Config.GitHubRepo + "/actions/workflows/" + deployWorkflow + "/runs?branch=" + mainBranch + "&per_page=1"
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cannabis-enforcement-tracker-backend")
	if w.Config.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+w.Config.GitHubToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Debug("deploy run unavailable", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Debug("deploy run unavailable", "status", resp.Status)
		return
	}
	var v struct {
		Runs []struct {
			HTMLURL    string    `json:"html_url"`
			Status     string    `json:"status"`
			Conclusion string    `json:"conclusion"`
			HeadSHA    string    `json:"head_sha"`
			StartedAt  time.Time `json:"run_started_at"`
			UpdatedAt  time.Time `json:"updated_at"`
		} `json:"workflow_runs"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&v); err != nil || len(v.Runs) == 0 {
		return
	}
	run := v.Runs[0]
	if !strings.HasPrefix(run.HTMLURL, "https://github.com/"+w.Config.GitHubRepo+"/actions/runs/") || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(run.HeadSHA) {
		return
	}
	out := &store.DeployRun{URL: run.HTMLURL, Status: run.Status, Conclusion: run.Conclusion, HeadSHA: run.HeadSHA, StartedAt: run.StartedAt.Unix(), UpdatedAt: run.UpdatedAt.Unix()}
	var batch string
	if w.Store.DB.QueryRowContext(ctx, "SELECT commit_sha FROM publication_batches ORDER BY id DESC LIMIT 1").Scan(&batch) == nil && batch != "" {
		repo := w.Config.RepoPath
		if _, e := gitOutput(ctx, repo, "cat-file", "-e", run.HeadSHA+"^{commit}"); e != nil {
			_ = gitQuiet(ctx, repo, 2*time.Minute, "fetch", "--quiet", "origin")
		}
		_, e := gitOutput(ctx, repo, "merge-base", "--is-ancestor", batch, run.HeadSHA)
		out.IncludesBatch = e == nil
	}
	w.Store.SetDeployRun(out)
}
