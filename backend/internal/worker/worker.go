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
	"gopkg.in/yaml.v3"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Worker struct {
	Store  *store.Store
	Config config.Config
	Nostr  *nostr.Service
	Health *research.Health
	// FetchImage downloads a photo for the duplicate check; tests replace it
	FetchImage func(context.Context, string) ([]byte, error)
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
	// several requests are worked on at once (WORKER_CONCURRENCY); each loop claims its own job,
	// and a claim is one atomic update, so two loops never take the same one
	loops := max(1, w.Config.WorkerConcurrency)
	var wg sync.WaitGroup
	// job storage: build leftovers and finished requests' workspaces are cleaned up
	wg.Add(1)
	go func() { defer wg.Done(); w.janitor(ctx) }()
	for i := 0; i < loops; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); w.jobs(ctx, "") }()
	}
	// and a lane of its own for committing validated drafts, so a finished draft is committed (and
	// pushed) within seconds instead of waiting for a research run to end
	wg.Add(1)
	go func() { defer wg.Done(); w.jobs(ctx, "publish") }()
	wg.Wait()
}

// jobs claims and processes queued jobs (of one kind when kind is set) one after another until the
// context ends.
func (w *Worker) jobs(ctx context.Context, kind string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:

			j, err := w.Store.ClaimKind(ctx, "jobs", kind, 2*time.Minute)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				slog.Error("job claim failed")
				continue
			}
			jobCtx, cancel := context.WithTimeout(ctx, w.Config.ResearchTimeout)
			stop := make(chan struct{})
			go w.heartbeat(jobCtx, "jobs", j, stop, cancel)
			err = w.process(jobCtx, j)
			close(stop)
			cancel()
			if ctx.Err() != nil {
				err = store.ErrInterrupted
			}
			if err != nil && j.Attempts >= 8 && !errors.Is(err, store.ErrInterrupted) && !errors.Is(err, store.ErrBudget) && !errors.Is(err, store.ErrConfiguration) {
				_ = w.Store.Transition(ctx, j.Ref, "failed", "Processing exhausted its retry budget. Restart this submission on localhost to try again.", "", "")
			}
			finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
			finishErr := w.Store.Finish(finishCtx, "jobs", j, err)
			if finishErr == nil && err != nil {
				w.waitingToRetry(finishCtx, j, err)
			}
			if finishErr == nil {
				if cleanupErr := w.cleanWorkspaces(finishCtx, j.Ref); cleanupErr != nil && finishCtx.Err() == nil {
					slog.Warn("completed job workspace cleanup incomplete", "error", cleanupErr)
				}
			}
			finishCancel()
			if finishErr == nil {
				if archiveErr := w.archiveScreenshots(ctx, j.Ref); archiveErr != nil && ctx.Err() == nil {
					slog.Warn("completed case screenshot archival incomplete", "error", archiveErr)
				}
			}
			if finishErr != nil {
				slog.Error("job completion failed", "job", j.ID)
			}

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

type report struct {
	Slug    string `json:"slug"`
	Country string `json:"country"`
	Date    string `json:"date"`
	Title   string `json:"title"`
	Sources []struct {
		URL string `yaml:"url" json:"url"`
	} `json:"sources"`
}

func Coverage(repo string) ([]report, error) {
	out := []report{}
	err := filepath.WalkDir(filepath.Join(repo, "reports"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parts := strings.SplitN(string(b), "---", 3)
		if len(parts) != 3 {
			return errors.New("invalid report front matter")
		}
		var r report
		if err = yaml.Unmarshal([]byte(parts[1]), &r); err != nil {
			return err
		}
		r.Slug = strings.TrimSuffix(d.Name(), ".md")
		out = append(out, r)
		return nil
	})
	return out, err
}
func (w *Worker) process(ctx context.Context, j store.Job) error {
	if j.Kind == "publish" {
		return w.publish(ctx, j)
	}
	var publicationQueued int
	if err := w.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE kind='publish' AND ref=?", j.Ref).Scan(&publicationQueued); err != nil {
		return err
	}
	if publicationQueued > 0 {
		return nil
	}

	var u, status, pr string
	if err := w.Store.DB.QueryRowContext(ctx, "SELECT canonical_url,status,pr_url FROM cases WHERE id=?", j.Ref).Scan(&u, &status, &pr); err != nil {
		return err
	}
	if store.Terminal(status) {
		return nil
	}
	if w.Config.Demo {
		for _, state := range []string{"fetching", "classifying", "researching"} {
			if err := w.Store.Transition(ctx, j.Ref, state, "Demonstration worker: no source research or publication has occurred.", "", ""); err != nil {
				return err
			}
		}
		return w.Store.Transition(ctx, j.Ref, w.uncertainOutcome(), "Demonstration complete. No AI research or publication occurred.", "", "")
	}
	var checkpoint researchCheckpoint
	var saved string
	err := w.Store.DB.QueryRowContext(ctx, "SELECT payload FROM research_checkpoints WHERE case_id=?", j.Ref).Scan(&saved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var result research.Result
	var initial safeurl.Evidence
	var dir string
	if err == nil {
		if err = json.Unmarshal([]byte(saved), &checkpoint); err != nil {
			return fmt.Errorf("invalid research checkpoint: %w", err)
		}
		result, initial, dir = checkpoint.Result, checkpoint.Initial, checkpoint.Dir
		if len(checkpoint.Evidence) == 0 {
			checkpoint.Evidence = research.ReadToolEvidence(dir)
		}
		result.CreatedByModel = checkpoint.Model
		initial.ArtifactDir = checkpoint.ArtifactDir
		// Build in a fresh trusted snapshot: the previous checkout may have been
		// interrupted after writing/moving the draft, or the site may have changed.
		dir, err = (research.Runner{Config: w.Config, Store: w.Store}).Snapshot(ctx, j.Ref)
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
		reports, err := Coverage(w.Config.RepoPath)
		if err != nil {
			return err
		}
		for _, r := range reports {
			for _, source := range r.Sources {
				known, err := safeurl.Normalize(source.URL)
				if err == nil && known == u {
					return w.Store.Transition(ctx, j.Ref, "duplicate", "This source is already cited in an existing report.", r.Slug, "")
				}
			}
		}
		if err = w.Store.Transition(ctx, j.Ref, "fetching", "Opening the submitted source.", "", ""); err != nil {
			return err
		}
		initial, err = w.loadSource(ctx, j.Ref, u)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		sourceCheck, _ := safeurl.Check(initial)
		if b, e := json.Marshal(sourceCheck); e == nil && (initial.Content != "" || initial.Rendered || initial.BrowserWarning != "") {
			_ = w.Store.SetCaseOutput(ctx, j.Ref, map[string]string{"source_check": string(b)})
		}
		if errors.Is(err, safeurl.ErrUnsafeDestination) {
			return w.Store.Transition(ctx, j.Ref, "insufficient_evidence", "The source destination is not permitted for public research.", "", "")
		}
		// The source check: the submitted page itself must load, be readable and be about cannabis before
		// any AI research. A report rests on the source that was sent, not on whatever else can be found.
		if err != nil {
			return w.Store.Transition(ctx, j.Ref, "source_unavailable", "The submitted page could not be loaded: "+err.Error()+". Send a link that opens without a login or paywall, or another report of the same event.", "", "")
		}
		check, err := safeurl.Check(initial)
		if b, jerr := json.Marshal(check); jerr == nil {
			_ = w.Store.SetCaseOutput(ctx, j.Ref, map[string]string{"source_check": string(b)})
		}
		if err != nil {
			return w.Store.Transition(ctx, j.Ref, "source_unavailable", "The submitted page loaded, but "+err.Error()+". Send a link to a page with the article text.", "", "")
		}
		// Vocabulary is an advisory hint, not a complete classifier of every language.
		// OpenCode determines relevance from the opened article even when no term matched.
		if err = w.Store.Transition(ctx, j.Ref, "classifying", check.Describe(), "", ""); err != nil {
			return err
		}
		run, err := w.Store.Reserve(ctx, j.Ref, "", w.Config.RunCents, w.Config.DailyCents)
		if err != nil {
			return err
		}
		if _, err = w.Store.DB.ExecContext(ctx, "UPDATE research_runs SET model=?,runner=?,status='preparing' WHERE id=?", w.Config.ResearchModel, w.Config.ResearchRunner, run); err != nil {
			return err
		}
		invoked := false
		defer func() {
			if !invoked {
				end, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = w.Store.EndRun(end, run, "failed", "")
				_ = w.Store.AddAIEvent(end, j.Ref, run, "system", "error", "The research workspace could not be prepared. The worker will retry automatically.")
			}
		}()
		runner := research.Runner{Config: w.Config, Store: w.Store}
		dir, err = runner.Snapshot(ctx, j.Ref)
		if err != nil {
			return err
		}
		if err = research.SaveEvidence(dir, []safeurl.Evidence{initial}); err != nil {
			return err
		}
		if err = w.Store.Transition(ctx, j.Ref, "researching", "OpenCode is researching this event and preparing a cited report automatically.", "", ""); err != nil {
			return err
		}
		invoked = true
		result, err = runner.Run(ctx, j.Ref, dir, run, initial, []byte(store.JSON(reports)))
		if err != nil {
			return err
		}
		result.CreatedByModel = w.Config.ResearchModel

		checkpoint = researchCheckpoint{Result: result, Initial: initial, Evidence: research.ReadToolEvidence(dir), Dir: dir, Model: result.CreatedByModel, ArtifactDir: initial.ArtifactDir}
		if _, err = w.Store.DB.ExecContext(ctx, "INSERT INTO research_checkpoints(case_id,payload,created_at) VALUES(?,?,?) ON CONFLICT(case_id) DO UPDATE SET payload=excluded.payload,created_at=excluded.created_at", j.Ref, store.JSON(checkpoint), time.Now().Unix()); err != nil {
			return err
		}
	}
	// the answer is public: the request page shows it as a report, whatever happens to it next
	_ = w.Store.SetCaseOutput(ctx, j.Ref, map[string]string{"ai_result": store.JSON(result), "draft_path": "", "draft_markdown": ""})
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
	if err = result.Validate(ids); err != nil {
		return w.Store.Transition(ctx, j.Ref, "failed", "The AI result did not pass validation ("+err.Error()+") and cannot be published automatically.", "", "")
	}
	// Scope decisions do not draft facts. They must not fail quotation checks for
	// having no quotations, or download corroborating pages unnecessarily.
	if !result.Drafts() {
		if err = w.Store.SetCaseOutput(ctx, j.Ref, map[string]string{"validation": ""}); err != nil {
			return err
		}
		outcome := result.Outcome
		if outcome == "needs_review" {
			outcome = "insufficient_evidence"
		}
		return w.Store.Transition(ctx, j.Ref, outcome, result.Explanation, "", "")
	}

	// a blocked extra source or a missing quotation is noted and worked around; only a really bad
	// answer (nothing confirmed, or most quotations missing) stops here
	if err = w.Store.Transition(ctx, j.Ref, "verifying", "Checking every quotation against saved source captures; opening sources not already captured.", "", ""); err != nil {
		return err
	}
	confirmed, evidence, check, err := result.Confirm(ctx, initial, func(ctx context.Context, u string) (safeurl.Evidence, error) { return w.loadSource(ctx, j.Ref, u) }, checkpoint.Evidence...)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	_ = w.Store.SetCaseOutput(ctx, j.Ref, map[string]string{"validation": store.JSON(check)})
	if err != nil {
		return w.Store.Transition(ctx, j.Ref, w.uncertainOutcome(), "The facts could not be confirmed in the sources ("+err.Error()+").", "", "")
	}
	result = confirmed
	_ = w.Store.SetCaseOutput(ctx, j.Ref, map[string]string{"ai_result": store.JSON(result)})
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
		sourceCase, err := w.claimSource(ctx, j.Ref, e.URL, e.FinalURL, hex.EncodeToString(sum[:]))
		if err != nil {
			return err
		}
		if sourceCase != j.Ref && !known[e.URL] && !known[e.FinalURL] {
			return w.rejectSourceOverlap(ctx, j.Ref, sourceCase, e.URL, check)
		}
	}
	if err = w.Store.Transition(ctx, j.Ref, "drafted", "A cited draft is being checked against the repository rules.", "", ""); err != nil {
		return err
	}
	// a new report keeps the submission it came from (the first one for this case), for reference;
	// updates of existing reports keep theirs
	if err = w.Store.DB.QueryRowContext(ctx, "SELECT id FROM submissions WHERE case_id=? ORDER BY created_at, id LIMIT 1", j.Ref).Scan(&result.Submission); err != nil {
		return err
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
		return w.Store.Transition(ctx, j.Ref, "duplicate", "The existing report already cites these sources, so nothing was added.", result.ExistingReport, "")
	}
	if err != nil {
		var build *research.BuildError
		if errors.As(err, &build) {
			return w.Store.Transition(ctx, j.Ref, "failed", "The AI's draft did not pass the report check:\n\n"+build.Detail+"\n\nRestart to research and draft it again.", "", "")
		}
		return w.Store.Transition(ctx, j.Ref, "failed", "The draft could not be prepared for the report build ("+err.Error()+"). Restart to try again.", "", "")
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
	_ = w.Store.SetCaseOutput(ctx, j.Ref, map[string]string{"ai_result": store.JSON(result), "draft_path": draft.Path, "draft_markdown": draft.Content})
	digest := sha256.Sum256([]byte(draft.Content))
	payload := draftJob{Dir: dir, Rel: draft.Path, PreviousRel: draft.PreviousPath, Slug: draft.Slug, Title: draft.Title, Digest: hex.EncodeToString(digest[:]), SourceOnly: result.Outcome == "additional_source", Updates: updates}
	tx, err := w.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = store.Enqueue(tx, "jobs", "publish", j.Ref, store.JSON(payload), time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// Private durable output; never part of the public API. Validation and source checks rerun on recovery.
type researchCheckpoint struct {
	Result                  research.Result
	Initial                 safeurl.Evidence
	Evidence                []safeurl.Evidence `json:",omitempty"`
	Dir, Model, ArtifactDir string
}

type draftJob struct {
	Dir, Rel, PreviousRel, Slug, Title, Digest string
	SourceOnly                                 bool      `json:",omitempty"` // sources added to an existing report
	Updates                                    []fileJob `json:",omitempty"` // other existing reports changed
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
	sha, err := commitReport(ctx, w.Config.RepoPath, j.Ref, d)
	if err != nil {
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
	if w.Config.AutoPush {
		// the commit goes to GitHub straight away, so its link works; a failed push leaves the
		// commit in place and says so
		if err := pushRepo(ctx, w.Config.RepoPath); err != nil {
			explanation += " Pushing it to GitHub failed (" + err.Error() + "); it is in the local repository only until the next push."
		} else {
			explanation += " It was pushed to GitHub."
		}
	} else {
		explanation += " It is available in local development; pushing is separate."
	}
	return w.Store.Transition(ctx, j.Ref, "committed", explanation, d.Slug, "")
}

func (w *Worker) uncertainOutcome() string { return "insufficient_evidence" }

// holdsSources: the case statuses that keep their sources, as they make or made a report from them.
// A case that is only queued, still researching, stopped or rejected holds none.
var holdsSources = map[string]bool{"verifying": true, "drafted": true, "publishing": true, "committed": true, "published": true}

// claimSource records that this case uses a source and returns the case that holds it: this one, or
// another case that holds it (holdsSources). A source recorded by a case that does not hold it
// (queued, failed, rejected) passes to this case rather than blocking it.
func (w *Worker) claimSource(ctx context.Context, cid, url, finalURL, digest string) (string, error) {
	tx, err := w.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO source_urls(url,case_id,final_url,digest,fetched_at) VALUES(?,?,?,?,?) ON CONFLICT(url) DO UPDATE SET final_url=excluded.final_url,digest=excluded.digest,fetched_at=excluded.fetched_at", url, cid, finalURL, digest, time.Now().Unix()); err != nil {
		return "", err
	}
	var owner, status string
	if err = tx.QueryRowContext(ctx, "SELECT s.case_id,coalesce(c.status,'') FROM source_urls s LEFT JOIN cases c ON c.id=s.case_id WHERE s.url=?", url).Scan(&owner, &status); err != nil {
		return "", err
	}
	if owner != cid && !holdsSources[status] {
		if _, err = tx.ExecContext(ctx, "UPDATE source_urls SET case_id=? WHERE url=? AND case_id=?", cid, url, owner); err != nil {
			return "", err
		}
		owner = cid
	}
	return owner, tx.Commit()
}

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
	if errors.Is(cause, store.ErrInterrupted) {
		why = "Processing was interrupted (the service restarted); it will resume automatically"
	}
	if at > time.Now().Unix() {
		why += " around " + time.Unix(at, 0).UTC().Format("15:04") + " UTC"
	}
	_ = w.Store.Transition(ctx, j.Ref, "queued", why+".", "", "")
}
