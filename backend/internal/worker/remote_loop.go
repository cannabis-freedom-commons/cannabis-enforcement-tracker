package worker

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// RunRemote works for a backend through its worker API (docs/WORKERS.md) until ctx ends: it takes
// research jobs, researches them here and reports every step back. Publishing stays with the
// backend. w has no store; its configuration is this worker's own.
func (w *Worker) RunRemote(parent context.Context, c *Client) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	pool := newAgentPool(w.Config, w.Health)
	c.Repo = w.Config.RepoPath
	c.Presence = func() presence {
		p := presence{Runner: w.Config.ResearchRunner}
		for _, a := range w.Config.Agents() {
			p.Agents = append(p.Agents, agentInfo{a.Model, a.MaxWorkers})
		}
		if w.Health != nil {
			snapshot := w.Health.Snapshot()
			p.Health = &snapshot
		}
		return p
	}
	var wg sync.WaitGroup
	if len(pool.agents) > 0 {
		wg.Go(func() { pool.recoverAgents(ctx, w.Config, nil) })
	}
	wg.Go(func() { w.remoteJanitor(ctx) })
	// the backend hears from this worker even while none of its agents is ready to take a job
	wg.Go(func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			if err := c.Hello(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("the backend cannot be reached", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	for range max(1, w.Config.MaxWorkers()) {
		wg.Go(func() { w.remoteJobs(ctx, c, pool) })
	}
	wg.Wait()
}

func (w *Worker) remoteJobs(ctx context.Context, c *Client, pool *agentPool) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		selected := *w
		release := func() {}
		if len(pool.agents) > 0 {
			model, done, ok := pool.acquire()
			if !ok {
				continue
			}
			release = done
			selected.Config.ResearchModel = model
			selected.Health = w.Health.ForModel(model)
			selected.observeResearch = func(err error) { pool.observe(model, err, time.Now()) }
		}
		j, ok, err := c.Claim(ctx)
		if err != nil || !ok {
			release()
			if err != nil && ctx.Err() == nil {
				slog.Warn("could not take a job from the backend", "error", err)
			}
			continue
		}
		slog.Info("research job taken", "job", j.ID, "case", j.Ref, "attempt", j.Attempts, "model", selected.Config.ResearchModel)
		if err := syncRepo(ctx, w.Config.RepoPath); err != nil {
			slog.Warn("the reports clone could not be updated; researching with what it has", "error", err)
		}
		selected.Backend = c.Job(j)
		timed, cancelTimeout := context.WithTimeout(ctx, w.Config.ResearchTimeout)
		jobCtx, cancel := context.WithCancelCause(timed)
		stop := make(chan struct{})
		go remoteHeartbeat(jobCtx, c, j, stop, cancel)
		err = selected.process(jobCtx, j)
		release()
		close(stop)
		lost := errors.Is(err, errLeaseLost) || errors.Is(context.Cause(jobCtx), errLeaseLost)
		cancel(nil)
		cancelTimeout()
		switch {
		case errors.Is(err, errCoverageRequeued) || errors.Is(err, errHeldBack):
			// already back in the backend's queue
		case lost:
			slog.Warn("research job lost its lease; another attempt will take it", "job", j.ID, "case", j.Ref)
		default:
			if ctx.Err() != nil {
				err = store.ErrInterrupted
			}
			if err != nil && !errors.Is(err, store.ErrInterrupted) {
				slog.Error("research job failed", "job", j.ID, "case", j.Ref, "error", err)
			}
			finishCtx, finishCancel := context.WithTimeout(context.Background(), retryFor+30*time.Second)
			if finishErr := c.Finish(finishCtx, j, err); finishErr != nil {
				slog.Error("could not report the job's end to the backend", "job", j.ID, "error", finishErr)
			}
			finishCancel()
		}
		w.cleanCase(j.Ref, err != nil)
	}
}

// remoteHeartbeat renews the job's lease every 30 seconds; a lost lease stops the job.
func remoteHeartbeat(ctx context.Context, c *Client, j store.Job, stop <-chan struct{}, cancel context.CancelCauseFunc) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.Heartbeat(ctx, j); errors.Is(err, errLeaseLost) {
				cancel(errLeaseLost)
				return
			} else if err != nil && ctx.Err() == nil {
				slog.Warn("heartbeat failed", "job", j.ID, "error", err)
			}
		}
	}
}

var repoSync sync.Mutex

// syncRepo fast-forwards the worker's clone of the reports repository, so research sees the
// reports published since the last job.
func syncRepo(ctx context.Context, repo string) error {
	repoSync.Lock()
	defer repoSync.Unlock()
	pullCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(pullCtx, "git", "pull", "--ff-only", "--quiet")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(strings.TrimSpace(string(out)) + ": " + err.Error())
	}
	return nil
}

// cleanCase removes a case's workspaces and captures from this worker once its job ended: the
// backend has what it needs. A failed attempt keeps an interrupted OpenCode session (slimmed),
// which the next attempt here can continue.
func (w *Worker) cleanCase(cid string, failed bool) {
	root, err := filepath.Abs(w.Config.Workspaces)
	if err != nil {
		return
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		m := workspaceName.FindStringSubmatch(e.Name())
		if !e.IsDir() || m == nil || m[1] != cid {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if failed && research.HasSession(dir) {
			_ = research.SlimKeptSession(dir)
			continue
		}
		_ = os.RemoveAll(dir)
	}
	if !failed {
		_ = os.RemoveAll(filepath.Join(root, "sources", cid))
	}
}

// remoteJanitor removes what jobs left behind: kept sessions past their limits, and any case
// folder untouched for a day (a job that never came back here).
func (w *Worker) remoteJanitor(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		root, err := filepath.Abs(w.Config.Workspaces)
		if err == nil {
			now := time.Now()
			entries, _ := os.ReadDir(root)
			keep := keptSessions(root, entries, now)
			for _, e := range entries {
				info, err := e.Info()
				if err != nil || !e.IsDir() || workspaceName.FindStringSubmatch(e.Name()) == nil {
					continue
				}
				dir := filepath.Join(root, e.Name())
				if research.HasSession(dir) && keep[dir] {
					continue
				}
				if now.Sub(info.ModTime()) > 24*time.Hour {
					_ = os.RemoveAll(dir)
				}
			}
			sources, _ := os.ReadDir(filepath.Join(root, "sources"))
			for _, e := range sources {
				if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > 24*time.Hour {
					_ = os.RemoveAll(filepath.Join(root, "sources", e.Name()))
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
