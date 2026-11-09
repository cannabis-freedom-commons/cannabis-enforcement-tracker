package worker

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/nostr"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Worker struct {
	// Store is the database of the backend process; a separate worker has none and reaches the
	// backend through Backend instead.
	Store   *store.Store
	Backend Backend
	Config  config.Config
	Nostr   *nostr.Service
	Health  *research.Health
	// FetchImage downloads a photo for the duplicate check; tests replace it
	FetchImage      func(context.Context, string) ([]byte, error)
	observeResearch func(error)
}

// backend is how research reaches the database: Backend when set, else the store.
func (w *Worker) backend() Backend {
	if w.Backend != nil {
		return w.Backend
	}
	return Local{w.Store}
}

func (w *Worker) Run(parent context.Context) {
	ctx, cancelWorkers := context.WithCancel(parent)
	defer cancelWorkers()
	outboxDone := make(chan struct{})
	if w.Nostr == nil {
		close(outboxDone)
	}
	defer func() {
		cancelWorkers()
		<-outboxDone
		if w.Nostr != nil {
			w.Nostr.Wait()
		}
	}()

	if w.Nostr != nil {
		w.Nostr.Listen(ctx)
		go func() { defer close(outboxDone); w.outbox(ctx) }()
	}
	// Research loops share one random allocator with independent per-agent limits.
	// With remote workers (RESEARCH_MODE=remote) the backend researches nothing itself.
	loops := w.Config.MaxWorkers()
	if w.Config.Remote() {
		loops = 0
	}
	pool := newAgentPool(w.Config, w.Health)
	var wg sync.WaitGroup
	if !w.Config.Demo && !w.Config.Remote() && len(pool.agents) > 0 {
		var log research.RunLog
		if w.Store != nil {
			log = Local{w.Store}
		}
		wg.Go(func() { pool.recoverAgents(ctx, w.Config, log) })
	}
	// job storage: build leftovers and finished requests' workspaces are cleaned up
	wg.Add(1)
	go func() { defer wg.Done(); w.janitor(ctx) }()
	for i := 0; i < loops; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); w.jobs(ctx, "research", pool) }()
	}
	// and a lane of its own for committing validated drafts, so a finished draft is committed (and
	// pushed) within seconds instead of waiting for a research run to end
	wg.Add(1)
	go func() { defer wg.Done(); w.jobs(ctx, "publish", nil) }()
	if w.Config.RepoSync {
		wg.Go(func() { syncLoop(ctx, w.Config.RepoPath) })
	}
	wg.Wait()
}

// finishJob ends a job after an attempt: the lease is released (to retry later when err is set),
// a request out of retries is marked failed, and the case's workspaces and screenshots are tidied
// (clean: the workspaces are on this machine). A job replaced by a review or held back is already
// back in the queue. The backend calls this for remote workers' jobs too.
func (w *Worker) finishJob(ctx context.Context, j store.Job, err error, clean bool) {
	if errors.Is(err, errCoverageRequeued) || errors.Is(err, errHeldBack) {
		// The fenced job was atomically replaced by a research review, or put back to wait
		// for a related submission.
		return
	}
	if err != nil && !errors.Is(err, store.ErrBudget) && !errors.Is(err, store.ErrConfiguration) && !errors.Is(err, store.ErrInterrupted) {
		// Private operator diagnostics, never part of the public AI transcript.
		slog.Error("job processing failed", "job", j.ID, "kind", j.Kind, "case", j.Ref, "attempt", j.Attempts, "error", err)
	}
	if err != nil && j.Attempts >= 8 && !errors.Is(err, store.ErrInterrupted) && !errors.Is(err, store.ErrBudget) && !errors.Is(err, store.ErrConfiguration) && !errors.Is(err, store.ErrResearchAgent) {
		_ = w.Store.Transition(ctx, j.Ref, "failed", "Processing exhausted its retry budget. Restart this submission on localhost to try again.", "", "")
	}
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finishCancel()
	finishErr := w.Store.Finish(finishCtx, "jobs", j, err)
	if finishErr == nil && err != nil {
		w.waitingToRetry(finishCtx, j, err)
	}
	if finishErr == nil && clean {
		if cleanupErr := w.cleanWorkspaces(finishCtx, j.Ref); cleanupErr != nil && finishCtx.Err() == nil {
			slog.Warn("completed job workspace cleanup incomplete", "error", cleanupErr)
		}
	}
	if finishErr == nil {
		if archiveErr := w.archiveScreenshots(ctx, j.Ref); archiveErr != nil && ctx.Err() == nil {
			slog.Warn("completed case screenshot archival incomplete", "error", archiveErr)
		}
	}
	if finishErr != nil {
		slog.Error("job completion failed", "job", j.ID)
	}
}

// jobs claims and processes queued jobs (of one kind when kind is set) one after another until the
// context ends.
func (w *Worker) jobs(ctx context.Context, kind string, pool *agentPool) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:

			selected := *w
			release := func() {}
			if pool != nil && len(pool.agents) > 0 && !w.Config.Demo {
				model, done, ok := pool.acquire()
				if !ok {
					continue
				}
				release = done
				selected.Config.ResearchModel = model
				selected.Health = w.Health.ForModel(model)
				selected.observeResearch = func(err error) { pool.observe(model, err, time.Now()) }
			}
			j, err := w.Store.ClaimKind(ctx, "jobs", kind, 2*time.Minute)
			if errors.Is(err, sql.ErrNoRows) {
				release()
				continue
			}
			if err != nil {
				release()
				slog.Error("job claim failed")
				continue
			}
			jobCtx, cancel := context.WithTimeout(ctx, w.Config.ResearchTimeout)
			stop := make(chan struct{})
			go w.heartbeat(jobCtx, "jobs", j, stop, cancel)
			err = selected.process(jobCtx, j)
			release()
			close(stop)
			cancel()
			if ctx.Err() != nil && !errors.Is(err, errCoverageRequeued) && !errors.Is(err, errHeldBack) {
				err = store.ErrInterrupted
			}
			w.finishJob(ctx, j, err, true)

		}
	}
}
func (w *Worker) heartbeat(ctx context.Context, table string, j store.Job, stop <-chan struct{}, cancel context.CancelFunc) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			if w.Store.Heartbeat(ctx, table, j, 2*time.Minute) != nil {
				cancel()
				return
			}
		}
	}
}
func (w *Worker) outbox(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j, err := w.Store.Claim(ctx, "outbox", 2*time.Minute)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				slog.Error("outbox claim failed")
				continue
			}
			delivery, cancel := context.WithTimeout(ctx, time.Minute)
			err = w.Nostr.Deliver(delivery, j)
			cancel()
			if finishErr := w.Store.Finish(ctx, "outbox", j, err); finishErr != nil {
				slog.Error("outbox completion failed")
			}
		}
	}
}

type report = research.ReportIndex

// Coverage indexes every report of the repository (or a job's snapshot of it).
func Coverage(repo string) ([]report, error) { return research.LoadReports(repo) }

func (w *Worker) process(ctx context.Context, j store.Job) error {
	if j.Kind == "publish" {
		return w.publish(ctx, j)
	}
	db := w.backend()
	u, status, publicationQueued, err := db.CaseSource(ctx, j.Ref)
	if err != nil {
		return err
	}
	if publicationQueued {
		return nil
	}
	if store.Terminal(status) {
		return nil
	}
	if w.Config.Demo {
		for _, state := range []string{"fetching", "classifying", "researching"} {
			if err := db.Transition(ctx, j.Ref, state, "Demonstration worker: no source research or publication has occurred.", "", ""); err != nil {
				return err
			}
		}
		return db.Transition(ctx, j.Ref, w.uncertainOutcome(), "Demonstration complete. No AI research or publication occurred.", "", "")
	}
	var checkpoint researchCheckpoint
	saved, err := db.Checkpoint(ctx, j.Ref)
	if err != nil {
		return err
	}
	var result research.Result
	var initial safeurl.Evidence
	var dir string
	// An OpenCode session kept open for one in-job correction (local runner only).
	var session *research.Session
	defer func() { session.Close() }()
	var baseEvidence []safeurl.Evidence
	if saved != "" {
		if err = json.Unmarshal([]byte(saved), &checkpoint); err != nil {
			return fmt.Errorf("invalid research checkpoint: %w", err)
		}
		if len(checkpoint.Evidence) == 0 {
			checkpoint.Evidence = research.ReadToolEvidence(checkpoint.Dir)
		}
		if checkpoint.Result.Outcome == "new_event" {
			if checkpoint.Coverage == nil {
				// Recover legacy inventories from the original checkout, never
				// from the fresh recovery snapshot.
				original, e := Coverage(checkpoint.Dir)
				if e != nil {
					checkpoint.ReviewCoverage = true
				} else {
					checkpoint.Coverage = eventCoverage(original, checkpoint.Result)
				}
			}
			current, e := Coverage(w.Config.RepoPath)
			if e != nil {
				return e
			}
			if coverageDiffers(current, checkpoint.Result, checkpoint.Coverage, nil) {
				checkpoint.ReviewCoverage = true
			}
			_, aliases, aliasErr := research.ReadReportAliases(w.Config.RepoPath)
			if aliasErr != nil {
				return aliasErr
			}
			if aliases != checkpoint.AliasDigest {
				checkpoint.ReviewCoverage = true
			}
		}
	}
	if saved != "" && !checkpoint.ReviewCoverage {
		result, initial, dir = checkpoint.Result, checkpoint.Initial, checkpoint.Dir
		if len(checkpoint.Evidence) == 0 {
			checkpoint.Evidence = research.ReadToolEvidence(dir)
		}
		result.CreatedByModel = checkpoint.Model
		if result.Outcome == "new_event" {
			result.AddedAt, result.Code = checkpoint.AddedAt, checkpoint.Code
		}
		initial.ArtifactDir = checkpoint.ArtifactDir
		// Build in a fresh trusted snapshot: the previous checkout may have been
		// interrupted after writing/moving the draft, or the site may have changed.
		dir, err = (research.Runner{Config: w.Config, Store: db}).Snapshot(ctx, j.Ref)
		if err != nil {
			return err
		}
		if err = research.SaveEvidence(dir, append([]safeurl.Evidence{initial}, checkpoint.Evidence...)); err != nil {
			return err
		}
	} else {
		if !w.Config.Demo && (!w.Config.ResearchEnabled() || (w.Health != nil && !w.Health.Ready())) {
			return store.ErrConfiguration
		}
		if checkpoint.ReviewCoverage {
			initial = checkpoint.Initial
			initial.ArtifactDir = checkpoint.ArtifactDir
		} else {
			reports, err := Coverage(w.Config.RepoPath)
			if err != nil {
				slog.Error("report coverage could not be loaded", "error", err)
				return err
			}
			for _, r := range reports {
				for _, source := range r.Sources {
					known, err := safeurl.Normalize(source.URL)
					if err == nil && known == u {
						return db.Transition(ctx, j.Ref, "duplicate", "This source is already cited in an existing report.", r.Slug, "")
					}
				}
			}
			if err = db.Transition(ctx, j.Ref, "fetching", "Opening the submitted source.", "", ""); err != nil {
				return err
			}
			initial, err = w.loadSource(ctx, j.Ref, u)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			sourceCheck, _ := safeurl.Check(initial)
			if b, e := json.Marshal(sourceCheck); e == nil && (initial.Content != "" || initial.Rendered || initial.BrowserWarning != "") {
				_ = db.SetCaseOutput(ctx, j.Ref, map[string]string{"source_check": string(b)})
			}
			if errors.Is(err, safeurl.ErrUnsafeDestination) {
				return db.Transition(ctx, j.Ref, "insufficient_evidence", "The source destination is not permitted for public research.", "", "")
			}
			if captureStorageError(err) {
				return err // retry local storage failures rather than blaming the source
			}
			// The source check: the submitted page itself must load, be readable and be about cannabis before
			// any AI research. A report rests on the source that was sent, not on whatever else can be found.
			if err != nil {
				return db.Transition(ctx, j.Ref, "source_unavailable", "The submitted page could not be loaded: "+err.Error()+". Send a link that opens without a login or paywall, or another report of the same event.", "", "")
			}
			check, err := safeurl.Check(initial)
			if b, jerr := json.Marshal(check); jerr == nil {
				_ = db.SetCaseOutput(ctx, j.Ref, map[string]string{"source_check": string(b)})
			}
			if err != nil {
				return db.Transition(ctx, j.Ref, "source_unavailable", "The submitted page loaded, but "+err.Error()+". Send a link to a page with the article text.", "", "")
			}
			// Vocabulary is an advisory hint, not a complete classifier of every language.
			// OpenCode determines relevance from the opened article even when no term matched.
			if err = db.Transition(ctx, j.Ref, "classifying", check.Describe(), "", ""); err != nil {
				return err
			}
			// the same story already being researched from another source: wait for it (related_hold.go)
			headline := check.Title
			if check.Metadata != nil {
				headline = storyHeadline(check.Title, check.Metadata.Description, check.Metadata.SiteName)
			}
			if other, err := w.relatedRunning(ctx, j.Ref, headline); err != nil {
				slog.Warn("related submissions could not be checked", "case", j.Ref, "error", err)
			} else if other != "" {
				held, err := w.holdBack(ctx, j, other)
				if err != nil {
					return err
				}
				if held {
					return errHeldBack
				}
			}
		}
		run, err := db.Reserve(ctx, j.Ref, w.Config.RunCents, w.Config.DailyCents)
		if err != nil {
			return err
		}
		preparing := "preparing"
		if err = db.UpdateRun(ctx, run, store.RunUpdate{Model: &w.Config.ResearchModel, Runner: &w.Config.ResearchRunner, Status: &preparing}); err != nil {
			return err
		}
		invoked := false
		defer func() {
			if !invoked {
				end, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = db.EndRun(end, run, "failed", "")
				_ = db.AddAIEvent(end, j.Ref, run, "system", "error", "The research workspace could not be prepared. The worker will retry automatically.")
			}
		}()
		runner := research.Runner{Config: w.Config, Store: db, ContextMode: research.ContextModeFor(w.Config.ResearchContext, j.Ref)}
		if checkpoint.ReviewCoverage {
			runner.PreviousResult = &checkpoint.Result
			runner.PreviousEvidence = checkpoint.Evidence
			runner.ReviewProblem = checkpoint.ReviewProblem
		}
		// An interrupted session continues where it stopped, in its own workspace snapshot;
		// anything that changed meanwhile is caught by the coverage checks below.
		resumed := w.resumableWorkspace(j.Ref)
		if resumed != "" {
			dir = resumed
			if err = runner.RestoreCheckout(ctx, j.Ref, dir); err != nil {
				return err
			}
		} else {
			dir, err = runner.Snapshot(ctx, j.Ref)
			if err != nil {
				return err
			}
			if err = research.SaveEvidence(dir, append([]safeurl.Evidence{initial}, checkpoint.Evidence...)); err != nil {
				return err
			}
		}
		explanation := "OpenCode is researching this event and preparing a cited report automatically."
		if resumed != "" {
			explanation = "The research was interrupted. OpenCode is continuing the same research session from where it stopped."
		}
		if err = db.Transition(ctx, j.Ref, "researching", explanation, "", ""); err != nil {
			return err
		}
		// The index and original Markdown must describe the same snapshot. Fetching
		// the submitted page may have taken long enough for other reports to land.
		reports, err := Coverage(dir)
		if err != nil {
			return err
		}
		runner.ResultCheck = w.resultCheck(dir, initial, checkpoint.Evidence, checkpoint.AddedAt, checkpoint.Code)
		invoked = true
		if resumed != "" {
			result, session, err = runner.Resume(ctx, j.Ref, dir, run)
		} else {
			result, session, err = runner.Start(ctx, j.Ref, dir, run, initial, []byte(store.JSON(reports)))
		}
		if w.observeResearch != nil && ctx.Err() == nil {
			w.observeResearch(err)
		}
		if err != nil {
			return err
		}
		result.CreatedByModel = w.Config.ResearchModel
		if result.Outcome == "new_event" {
			result.AddedAt = checkpoint.AddedAt
			result.Code = checkpoint.Code
		} else {
			checkpoint.AddedAt, checkpoint.Code = "", ""
		}

		_, aliasDigest, aliasErr := research.ReadReportAliases(dir)
		if aliasErr != nil {
			return aliasErr
		}
		baseEvidence = checkpoint.Evidence
		checkpoint = researchCheckpoint{Result: result, Initial: initial, Evidence: append(append([]safeurl.Evidence(nil), baseEvidence...), research.ReadToolEvidence(dir)...), Dir: dir, Model: result.CreatedByModel, ArtifactDir: initial.ArtifactDir, Coverage: eventCoverage(reports, result), AliasDigest: aliasDigest, AddedAt: checkpoint.AddedAt, Code: checkpoint.Code, RepairAttempts: checkpoint.RepairAttempts}
		if err = db.SaveCheckpoint(ctx, j.Ref, store.JSON(checkpoint)); err != nil {
			return err
		}
	}
	// A rejected answer gets one correction. With an open session the same agent fixes it here,
	// and the corrected answer goes through every check again from the top of this loop.
	for {
		result.CleanPresentation()
		// the answer is public: the request page shows it as a report, whatever happens to it next
		_ = db.SetCaseOutput(ctx, j.Ref, map[string]string{"ai_result": store.JSON(result), "draft_path": "", "draft_markdown": ""})
		b, err := os.ReadFile(filepath.Join(w.Config.RepoPath, "src/data/countries.json"))
		if err != nil {
			return err
		}
		var countries []struct {
			ID string `json:"id"`
		}
		if err = json.Unmarshal(b, &countries); err != nil {
			return err
		}
		ids := map[string]bool{}
		for _, c := range countries {
			ids[c.ID] = true
		}
		// counts in the text that the figures miss are pointed out as open questions
		result.Uncertainties = append(result.Uncertainties, research.FigureGaps(result)...)
		result.Tags = research.CleanTags(result.Tags) // a tag in another script or with spaces is mended, not refused
		vocab, vocabErr := research.LoadVocabularies(w.Config.RepoPath)
		if vocabErr != nil && !os.IsNotExist(vocabErr) {
			return vocabErr
		}
		var vocabs []research.Vocabularies
		if vocabErr == nil {
			vocabs = append(vocabs, vocab)
		}
		if err = result.Validate(ids, vocabs...); err != nil {
			if checkpoint.RepairAttempts < 1 {
				problem := "The result did not pass schema validation: " + err.Error() + ". Correct the result using the saved sources and current reports; do not invent missing evidence."
				if session != nil {
					if result, err = w.correctInSession(ctx, j, session, &checkpoint, problem, dir, baseEvidence); err != nil {
						return err
					}
					continue
				}
				return w.requeueResearchReview(ctx, j, nil, problem)
			}
			return db.Transition(ctx, j.Ref, "failed", "The AI result did not pass validation ("+err.Error()+") and cannot be published automatically.", "", "")
		}
		// Scope decisions do not draft facts. They must not fail quotation checks for
		// having no quotations, or download corroborating pages unnecessarily.
		if !result.Drafts() {
			if err = db.SetCaseOutput(ctx, j.Ref, map[string]string{"validation": ""}); err != nil {
				return err
			}
			outcome := result.Outcome
			if outcome == "needs_review" {
				outcome = "insufficient_evidence"
			}
			matched := ""
			if outcome == "duplicate" && result.ExistingReport != "" {
				reports, err := Coverage(w.Config.RepoPath)
				if err != nil {
					return err
				}
				for _, report := range reports {
					if report.Slug == result.ExistingReport {
						matched = report.Slug
					}
				}
				if matched == "" {
					return db.Transition(ctx, j.Ref, "failed", "The proposed duplicate names a report that is no longer available. Restart to check the current coverage.", "", "")
				}
			}
			return db.Transition(ctx, j.Ref, outcome, result.Explanation, matched, "")
		}
		if result.Outcome == "new_event" {
			current, err := Coverage(w.Config.RepoPath)
			if err != nil {
				return err
			}
			if coverageDiffers(current, result, checkpoint.Coverage, nil) {
				return w.requeueCoverageReview(ctx, j, nil)
			}
			_, aliases, aliasErr := research.ReadReportAliases(w.Config.RepoPath)
			if aliasErr != nil {
				return aliasErr
			}
			if aliases != checkpoint.AliasDigest {
				return w.requeueCoverageReview(ctx, j, nil)
			}
		}

		// a blocked extra source or a missing quotation is noted and worked around; only a really bad
		// answer (nothing confirmed, or most quotations missing) stops here
		if err = db.Transition(ctx, j.Ref, "verifying", "Checking every quotation against saved source captures; opening sources not already captured.", "", ""); err != nil {
			return err
		}
		confirmed, evidence, check, err := result.Confirm(ctx, initial, func(ctx context.Context, u string) (safeurl.Evidence, error) { return w.loadSource(ctx, j.Ref, u) }, checkpoint.Evidence...)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = db.SetCaseOutput(ctx, j.Ref, map[string]string{"validation": store.JSON(check)})
		if err != nil {
			return db.Transition(ctx, j.Ref, w.uncertainOutcome(), "The facts could not be confirmed in the sources ("+err.Error()+").", "", "")
		}
		result = confirmed
		if result.Outcome == "new_event" {
			reports, identityErr := Coverage(dir)
			if identityErr != nil {
				return identityErr
			}
			identityErr = research.CheckEventComparisons(result, reports, true)
			if identityErr == nil {
				identityErr = research.CheckRetiredIdentity(dir, result.Slug, result.Code)
			}
			if identityErr != nil {
				if checkpoint.RepairAttempts < 1 {
					problem := "The new-event identity check failed: " + identityErr.Error() + ". Read each nominated report (tracker_get_report, or existing-reports/<slug>.md). Supply event_comparisons for every nominated match, with a body excerpt and a quotation-supported fact demonstrating the distinction. A matching event or later case stage must update the existing report instead of creating another. Keep genuinely distinct events and campaign components separate."
					if session != nil {
						if result, err = w.correctInSession(ctx, j, session, &checkpoint, problem, dir, baseEvidence); err != nil {
							return err
						}
						continue
					}
					return w.requeueResearchReview(ctx, j, nil, problem)
				}
				return db.Transition(ctx, j.Ref, "failed", "The proposed new report could not establish a supported distinction from existing coverage after automatic review. No new report was published.", "", "")
			}
		}
		_ = db.SetCaseOutput(ctx, j.Ref, map[string]string{"ai_result": store.JSON(result)})
		if err = research.SaveEvidence(dir, evidence); err != nil {
			return err
		}
		// a source already cited by a report this answer updates is not an overlap: adding to that
		// report is the point
		targets := map[string]bool{}
		if result.Outcome == "additional_source" {
			targets[result.ExistingReport] = true
		}
		for _, u := range result.Updates {
			targets[u.Report] = true
		}
		known := map[string]bool{}
		if len(targets) > 0 {
			reports, err := Coverage(w.Config.RepoPath)
			if err != nil {
				slog.Error("report coverage could not be loaded", "error", err)
				return err
			}
			for _, r := range reports {
				if targets[r.Slug] {
					for _, source := range r.Sources {
						if u, err := safeurl.Normalize(source.URL); err == nil {
							known[u] = true
						}
					}
				}
			}
		}
		for _, e := range evidence {
			sum := sha256.Sum256([]byte(e.Content))
			sourceCase, err := db.ClaimSource(ctx, j.Ref, e.URL, e.FinalURL, hex.EncodeToString(sum[:]))
			if err != nil {
				return err
			}
			if sourceCase != j.Ref && !known[e.URL] && !known[e.FinalURL] {
				return db.RejectSourceOverlap(ctx, j.Ref, sourceCase, e.URL, check)
			}
		}
		if err = db.Transition(ctx, j.Ref, "drafted", "A cited draft is being checked against the repository rules.", "", ""); err != nil {
			return err
		}
		// a new report keeps the submission it came from (the first one for this case), for reference;
		// updates of existing reports keep theirs
		if result.Submission, err = db.FirstSubmission(ctx, j.Ref); err != nil {
			return err
		}
		// a picture whose caption admits it is generic (file, stock, not known to show the event) is
		// left out; only this submission's new media is checked
		if len(result.Media) > 0 {
			var stock []string
			if result.Media, stock = research.DropStockMedia(result.Media); len(stock) > 0 {
				slog.Info("generic photos left out", "case", j.Ref, "count", len(stock))
			}
		}
		// the same photo republished by several outlets is shown once
		if len(result.Media) > 0 {
			fetch := w.FetchImage
			if fetch == nil {
				fetch = research.FetchImage
			}
			var existing []string
			if result.Outcome != "new_event" {
				existing = research.ReportMedia(dir, result.ExistingReport)
			}
			var dropped []string
			result.Media, dropped = research.DedupeMedia(ctx, result.Media, existing, fetch)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if len(dropped) > 0 {
				slog.Info("duplicate photos left out", "case", j.Ref, "count", len(dropped))
			}
		}
		draft, err := research.ValidateBuild(ctx, dir, result)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, research.ErrUnchanged) {
			return db.Transition(ctx, j.Ref, "duplicate", "The existing report already cites these sources, so nothing was added.", result.ExistingReport, "")
		}
		if err != nil {
			if errors.Is(err, research.ErrMergeTarget) && checkpoint.RepairAttempts < 1 {
				problem := "An update could not match its existing sentence. Read the current target report and copy each corroborated sentence exactly into existing_text, without citations. Only genuinely new details may use new_detail:true. Do not append paraphrases of existing prose."
				if session != nil {
					if result, err = w.correctInSession(ctx, j, session, &checkpoint, problem, dir, baseEvidence); err != nil {
						return err
					}
					continue
				}
				return w.requeueResearchReview(ctx, j, nil, problem)
			}
			var build *research.BuildError
			if errors.As(err, &build) {
				return db.Transition(ctx, j.Ref, "failed", "The AI's draft did not pass the report check:\n\n"+build.Detail+"\n\nRestart to research and draft it again.", "", "")
			}
			return db.Transition(ctx, j.Ref, "failed", "The draft could not be prepared for the report build ("+err.Error()+"). Restart to try again.", "", "")
		}
		// the request page lists the reports this run changes: keep only the updates that change something
		applied := map[string]bool{}
		updates := []fileJob{}
		for _, u := range draft.Updates {
			applied[u.Slug] = true
			sum := sha256.Sum256([]byte(u.Content))
			updates = append(updates, fileJob{Rel: u.Path, Slug: u.Slug, Title: u.Title, Digest: hex.EncodeToString(sum[:])})
		}
		kept := result.Updates[:0]
		for _, u := range result.Updates {
			if applied[u.Report] {
				kept = append(kept, u)
			}
		}
		result.Updates = kept
		_ = db.SetCaseOutput(ctx, j.Ref, map[string]string{"ai_result": store.JSON(result), "draft_path": draft.Path, "draft_markdown": draft.Content})
		digest := sha256.Sum256([]byte(draft.Content))
		payload := draftJob{Dir: dir, Rel: draft.Path, PreviousRel: draft.PreviousPath, Slug: draft.Slug, Title: draft.Title, Digest: hex.EncodeToString(digest[:]), SourceOnly: result.Outcome == "additional_source", Updates: updates, Coverage: checkpoint.Coverage, AliasDigest: checkpoint.AliasDigest, IdentityResult: &result}
		return db.QueuePublish(ctx, j, payload)
	}
}

// correctInSession sends one correction to the still-open OpenCode session. The checkpoint is
// marked for review first: if the worker stops during the correction, recovery runs the same
// correction as a fresh review from saved evidence and never accepts the rejected answer.
func (w *Worker) correctInSession(ctx context.Context, j store.Job, session *research.Session, checkpoint *researchCheckpoint, problem, dir string, baseEvidence []safeurl.Evidence) (research.Result, error) {
	checkpoint.ReviewCoverage, checkpoint.ReviewProblem = true, problem
	checkpoint.RepairAttempts++
	db := w.backend()
	if err := db.SaveCheckpoint(ctx, j.Ref, store.JSON(*checkpoint)); err != nil {
		return research.Result{}, err
	}
	_ = db.SetCaseOutput(ctx, j.Ref, map[string]string{"draft_path": "", "draft_markdown": ""})
	if err := db.Transition(ctx, j.Ref, "researching", "The proposed result needs a correction. OpenCode is fixing it in the same research session; all evidence and report checks still apply.", "", ""); err != nil {
		return research.Result{}, err
	}
	run, err := db.Reserve(ctx, j.Ref, w.Config.RunCents, w.Config.DailyCents)
	if err != nil {
		return research.Result{}, err
	}
	preparing := "preparing"
	if err = db.UpdateRun(ctx, run, store.RunUpdate{Model: &w.Config.ResearchModel, Runner: &w.Config.ResearchRunner, Status: &preparing, Workspace: &dir}); err != nil {
		return research.Result{}, err
	}
	result, err := session.Correct(ctx, run, problem)
	if w.observeResearch != nil && ctx.Err() == nil {
		w.observeResearch(err)
	}
	if err != nil {
		return research.Result{}, err
	}
	result.CreatedByModel = w.Config.ResearchModel
	addedAt, code := checkpoint.AddedAt, checkpoint.Code
	if result.Outcome == "new_event" {
		result.AddedAt, result.Code = addedAt, code
	} else {
		addedAt, code = "", ""
	}
	reports, err := Coverage(dir)
	if err != nil {
		return research.Result{}, err
	}
	_, aliasDigest, err := research.ReadReportAliases(dir)
	if err != nil {
		return research.Result{}, err
	}
	*checkpoint = researchCheckpoint{Result: result, Initial: checkpoint.Initial, Evidence: append(append([]safeurl.Evidence(nil), baseEvidence...), research.ReadToolEvidence(dir)...), Dir: dir, Model: result.CreatedByModel, ArtifactDir: checkpoint.ArtifactDir, Coverage: eventCoverage(reports, result), AliasDigest: aliasDigest, AddedAt: addedAt, Code: code, RepairAttempts: checkpoint.RepairAttempts}
	if err = db.SaveCheckpoint(ctx, j.Ref, store.JSON(*checkpoint)); err != nil {
		return research.Result{}, err
	}
	return result, nil
}

// Private durable output; never part of the public API. Validation and source checks rerun on recovery.
type researchCheckpoint struct {
	Result                  research.Result
	Initial                 safeurl.Evidence
	Evidence                []safeurl.Evidence `json:",omitempty"`
	Dir, Model, ArtifactDir string
	ReviewCoverage          bool   `json:",omitempty"`
	ReviewProblem           string `json:",omitempty"`
	RepairAttempts          int    `json:",omitempty"`
	Coverage                map[string]string
	AliasDigest             string `json:",omitempty"`
	AddedAt                 string `json:",omitempty"`
	Code                    string `json:",omitempty"`
}

type draftJob struct {
	Dir, Rel, PreviousRel, Slug, Title, Digest string
	SourceOnly                                 bool              `json:",omitempty"` // sources added to an existing report
	Updates                                    []fileJob         `json:",omitempty"` // other existing reports changed
	Coverage                                   map[string]string // reports seen by the model, retained through recovery
	AliasDigest                                string            `json:",omitempty"`
	IdentityResult                             *research.Result  `json:",omitempty"`
	// A draft a remote worker uploaded (Uploaded): the git blobs its changed reports started from
	// (in place of Dir's own git history), and its photos, downloaded by the worker into MediaDir
	// (in Dir; "" when there were none).
	Uploaded   bool              `json:",omitempty"`
	BaseBlobs  map[string]string `json:",omitempty"`
	MediaDir   string            `json:",omitempty"`
	MediaError string            `json:",omitempty"`
}

// fileJob is another existing report a run changes, checked like the main draft.
type fileJob struct{ Rel, Slug, Title, Digest string }

func (w *Worker) publish(ctx context.Context, j store.Job) error {
	var status string
	if err := w.Store.DB.QueryRowContext(ctx, "SELECT status FROM cases WHERE id=?", j.Ref).Scan(&status); err != nil {
		return err
	}
	if status == "committed" {
		return nil
	}
	var d draftJob
	if err := json.Unmarshal([]byte(j.Payload), &d); err != nil {
		return err
	}
	root, err := filepath.Abs(w.Config.Workspaces)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, d.Dir)
	if err != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return errors.New("invalid workspace")
	}
	b, err := os.ReadFile(filepath.Join(d.Dir, d.Rel))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != d.Digest {
		return errors.New("validated draft changed")
	}
	for _, u := range d.Updates {
		b, err := os.ReadFile(filepath.Join(d.Dir, u.Rel))
		if err != nil {
			return err
		}
		if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != u.Digest {
			return errors.New("validated report update changed")
		}
	}
	if err = w.Store.Transition(ctx, j.Ref, "publishing", "Committing the validated report to the local repository.", d.Slug, ""); err != nil {
		return err
	}
	// the new photos are downloaded first, outside the publication lock, and their thumbnails
	// committed with the report (local_media.go)
	rels := []string{d.Rel}
	for _, u := range d.Updates {
		rels = append(rels, u.Rel)
	}
	var media *mediaBatch
	var mediaErr error
	if d.Uploaded {
		media, mediaErr = uploadedMedia(d)
	} else {
		media, mediaErr = prepareMedia(ctx, w.Config.RepoPath, d.Dir, rels)
	}
	defer media.Close()
	sha, mediaNote, err := commitReportWithMedia(ctx, w.Config.RepoPath, j.Ref, d, media, w.Config.RepoSync)
	if err != nil {
		var changed *coverageChanged
		if errors.As(err, &changed) {
			return w.requeueCoverageReview(ctx, j, b)
		}
		return err
	}
	_, err = w.Store.DB.ExecContext(ctx, "INSERT INTO local_publications(case_id,commit_sha,report_path,created_at) VALUES(?,?,?,?) ON CONFLICT(case_id) DO UPDATE SET commit_sha=excluded.commit_sha,report_path=excluded.report_path", j.Ref, sha, d.Rel, time.Now().Unix())
	if err != nil {
		return err
	}
	explanation := "The report was committed to the local repository (" + sha[:12] + ")."
	if d.SourceOnly {
		explanation = "The source was added to the existing report “" + d.Title + "” and committed to the local repository (" + sha[:12] + ")."
	}
	if len(d.Updates) > 0 {
		titles := []string{}
		for _, u := range d.Updates {
			titles = append(titles, "“"+u.Title+"”")
		}
		explanation += " It also updated " + strings.Join(titles, ", ") + "."
	}
	if mediaErr != nil {
		explanation += " Photo thumbnails were not added (" + mediaErr.Error() + ")."
	} else if mediaNote != "" {
		explanation += " " + mediaNote
	}
	if w.Config.AutoPush {
		// the commit goes to GitHub straight away, so its link works; a failed push leaves the
		// commit in place and says so
		if err := pushRepo(ctx, w.Config.RepoPath, w.Config.RepoSync); err != nil {
			explanation += " Pushing it to GitHub failed (" + err.Error() + "); it is in the local repository only until the next push."
		} else {
			explanation += " It was pushed to GitHub."
			// a push after others' commits rebases this one: record the commit GitHub has
			if rebased := caseCommit(ctx, w.Config.RepoPath, j.Ref, d.Rel); rebased != "" && rebased != sha {
				explanation = strings.Replace(explanation, sha[:12], rebased[:12], 1)
				if _, err := w.Store.DB.ExecContext(ctx, "UPDATE local_publications SET commit_sha=? WHERE case_id=?", rebased, j.Ref); err != nil {
					return err
				}
			}
		}
	} else {
		explanation += " It is available in local development; pushing is separate."
	}
	return w.Store.Transition(ctx, j.Ref, "committed", explanation, d.Slug, "")
}

// uncertainStatus is where a submission ends when its facts cannot be established.
const uncertainStatus = "insufficient_evidence"

func (w *Worker) uncertainOutcome() string { return uncertainStatus }

// waitingToRetry shows a request whose attempt failed or was interrupted, and whose job waits to run
// again, as queued: it is not researching while nothing works on it. Budget and configuration
// waits keep their own status, and a finished request (failed for good, rejected) is left as it is.
func (w *Worker) waitingToRetry(ctx context.Context, j store.Job, cause error) {
	if errors.Is(cause, store.ErrBudget) || errors.Is(cause, store.ErrConfiguration) {
		return
	}
	var status, jobStatus string
	var at int64
	if w.Store.DB.QueryRowContext(ctx, "SELECT c.status,j.status,j.available_at FROM cases c JOIN jobs j ON j.ref=c.id WHERE c.id=? AND j.id=?", j.Ref, j.ID).Scan(&status, &jobStatus, &at) != nil {
		return
	}
	if jobStatus != "queued" || !map[string]bool{"fetching": true, "classifying": true, "researching": true, "verifying": true, "drafted": true, "publishing": true}[status] {
		return
	}
	why := "The last attempt did not finish; it will be retried automatically"
	if errors.Is(cause, store.ErrResearchAgent) {
		why = "The research agent stopped before its first model step. The request will be retried automatically without using a case retry"
	}
	if errors.Is(cause, store.ErrInterrupted) {
		why = "Processing was interrupted (the service restarted); it will resume automatically"
	}
	if at > time.Now().Unix() {
		why += " around " + time.Unix(at, 0).UTC().Format("15:04") + " UTC"
	}
	_ = w.Store.Transition(ctx, j.Ref, "queued", why+".", "", "")
}
