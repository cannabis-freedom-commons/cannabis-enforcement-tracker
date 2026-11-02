package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// Only case workspaces are disposable. In particular, never descend into sources/.
var workspaceName = regexp.MustCompile(`^([0-9a-f]{32})-[0-9]+$`)

const defaultJobLogRetention = 14 * 24 * time.Hour

// CleanWorkspaces is also the online maintenance command. It leaves active cases,
// recent directories and queued publication inputs alone, and never changes the DB.
func (w *Worker) CleanWorkspaces(ctx context.Context) error {
	return w.cleanWorkspaces(ctx, "")
}

func (w *Worker) cleanWorkspaces(ctx context.Context, completedCase string) error {
	root, err := filepath.Abs(w.Config.Workspaces)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	retention := w.Config.JobLogRetention
	if retention <= 0 {
		retention = defaultJobLogRetention
	}
	now := time.Now()
	pruned, removed := 0, 0
	var failures []error
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		m := workspaceName.FindStringSubmatch(e.Name())
		if !e.IsDir() || m == nil || (completedCase != "" && completedCase != m[1]) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		// A periodic/online sweep must not race a just-created snapshot. The completion
		// hook can clean its own case immediately after its job lease has been finished.
		if completedCase == "" && now.Sub(info.ModTime()) < 10*time.Minute {
			continue
		}
		dir := filepath.Join(root, e.Name())
		var status string
		var updated int64
		var running bool
		err = w.Store.DB.QueryRowContext(ctx, `SELECT status,updated_at,
    EXISTS(SELECT 1 FROM jobs WHERE ref=cases.id AND status='running')
    FROM cases WHERE id=?`, m[1]).Scan(&status, &updated, &running)
		if errors.Is(err, sql.ErrNoRows) {
			if err = os.RemoveAll(dir); err == nil {
				removed++
			} else {
				failures = append(failures, err)
			}
			continue
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if running {
			continue
		}
		// A validated draft is the only checkout a later stage reuses. Research retries
		// rebuild from durable checkpoints in a fresh snapshot, not an old checkout.
		var payload string
		err = w.Store.DB.QueryRowContext(ctx, `SELECT payload FROM jobs WHERE ref=? AND kind='publish' AND status IN ('queued','running')`, m[1]).Scan(&payload)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			failures = append(failures, err)
			continue
		}
		if err == nil {
			var draft draftJob
			if err = json.Unmarshal([]byte(payload), &draft); err != nil || draft.Dir == "" {
				failures = append(failures, errors.New("invalid publication workspace; cleanup skipped"))
				continue
			}
			pending, err := filepath.Abs(draft.Dir)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if pending == dir {
				research.PruneBuild(dir)
				continue
			}
		}
		// The receipts, public activity, usage and retry checkpoints remain in SQLite.
		// Retention applies only to finished cases' private logs/evidence in job folders.
		if store.Terminal(status) && now.Sub(time.Unix(updated, 0)) >= retention && now.Sub(info.ModTime()) >= retention {
			if err = os.RemoveAll(dir); err == nil {
				removed++
			} else {
				failures = append(failures, err)
			}
			continue
		}
		changed, err := pruneIfNeeded(dir)
		if err != nil {
			failures = append(failures, err)
		} else if changed {
			pruned++
		}
	}
	if pruned+removed > 0 {
		slog.Info("job workspaces cleaned", "pruned", pruned, "removed", removed)
	}
	return errors.Join(failures...)
}

// pruneIfNeeded keeps raw logs and evidence, removing the checkout and caches.
func pruneIfNeeded(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() || !research.KeptFile(e.Name()) {
			if err := research.PruneWorkspace(dir); err != nil {
				return false, fmt.Errorf("prune job workspace: %w", err)
			}
			return true, nil
		}
	}
	return false, nil
}

// janitor also catches workspaces left behind by crashes or interrupted cleanup.
func (w *Worker) janitor(ctx context.Context) {
	clean := func() {
		if err := w.CleanWorkspaces(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("job workspace cleanup incomplete", "error", err)
		}
	}
	clean()
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			clean()
		}
	}
}

// Archive only completed cases; research and publication retain the original PNG
// until no job can still be reading it. Conversion failure leaves the PNG intact.
func (w *Worker) archiveScreenshots(ctx context.Context, cid string) error {
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(cid) {
		return errors.New("invalid archive case")
	}
	var status string
	var running bool
	if err := w.Store.DB.QueryRowContext(ctx, `SELECT status, EXISTS(SELECT 1 FROM jobs WHERE ref=cases.id AND status='running') FROM cases WHERE id=?`, cid).Scan(&status, &running); err != nil {
		return err
	}
	if running || !store.Terminal(status) {
		return nil
	}
	root, err := filepath.Abs(w.Config.Workspaces)
	if err != nil {
		return err
	}
	dirs := []string{}
	sources := filepath.Join(root, "sources", cid)
	if _, err := os.Stat(sources); err == nil {
		dirs = append(dirs, sources)
	} else if !os.IsNotExist(err) {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if m := workspaceName.FindStringSubmatch(e.Name()); e.IsDir() && m != nil && m[1] == cid {
			dirs = append(dirs, filepath.Join(root, e.Name()))
		}
	}
	script, err := filepath.Abs(filepath.Join(w.Config.RepoPath, "scripts", "archive-screenshots.mjs"))
	if err != nil {
		return err
	}
	run, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var failures []error
	for _, dir := range dirs {
		command := exec.CommandContext(run, "node", script, dir)
		command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
		command.WaitDelay = 5 * time.Second
		if err := command.Run(); err != nil {
			failures = append(failures, fmt.Errorf("archive capture files (originals retained on failure): %w", err))
		}
	}
	return errors.Join(failures...)
}
