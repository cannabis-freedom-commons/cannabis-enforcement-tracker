package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/frontmatter"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"gopkg.in/yaml.v3"
)

type coverageChanged struct{ country string }

func (e *coverageChanged) Error() string {
	return fmt.Sprintf("report coverage changed for %s since research; a fresh event-identity check is required", e.country)
}

// Called under the Git publication lock. Comparing entire report contents also
// catches corrected dates, added sources and renamed reports. Country is a
// conservative scope, not an event fingerprint: matching dates/figures/places
// alone never establish that two incidents are the same.
func checkPublicationCoverage(repo string, d draftJob, content []byte) error {
	parts := frontmatter.Split(string(content))
	if len(parts) != 3 {
		return errors.New("invalid draft front matter")
	}
	var draft struct{ Country string }
	if err := yaml.Unmarshal([]byte(parts[1]), &draft); err != nil {
		return err
	}
	// Incident metadata is enforced by CheckReport before publication.
	if draft.Country == "" || d.SourceOnly {
		return nil
	}
	seen := d.Coverage
	if seen == nil {
		before, err := Coverage(d.Dir)
		if err != nil {
			return err
		}
		seen = countryCoverage(before, draft.Country)
	}
	current, err := Coverage(repo)
	if err != nil {
		return err
	}
	// Incidental updates are already checked against their original Git blobs by
	// commitReport; their workspace copies contain the proposed changes.
	updates := map[string]bool{}
	for _, u := range d.Updates {
		updates[u.Slug] = true
	}
	if coverageDiffers(current, draft.Country, seen, updates) {
		return &coverageChanged{country: draft.Country}
	}
	return nil
}

func countryCoverage(reports []report, country string) map[string]string {
	out := map[string]string{}
	for _, r := range reports {
		if r.Country == country {
			out[r.Slug] = r.Digest
		}
	}
	return out
}

func coverageDiffers(reports []report, country string, before map[string]string, updates map[string]bool) bool {
	for _, r := range reports {
		if r.Country == country && !updates[r.Slug] && before[r.Slug] != r.Digest {
			return true
		}
	}
	return false
}

var errCoverageRequeued = errors.New("publication replaced by coverage review")

// Replace a stale research/publication decision atomically, fencing its lease.
// A crash at any point resumes either the original publication or this review;
// it never publishes the stale draft or loses its completed research/evidence.
func (w *Worker) requeueCoverageReview(ctx context.Context, j store.Job, content []byte) error {
	return w.requeueResearchReview(ctx, j, content, "")
}

// A malformed result or unmatched merge target gets one correction attempt with saved
// evidence. The service still runs all schema, quotation, merge and publication checks.
func (w *Worker) requeueResearchReview(ctx context.Context, j store.Job, content []byte, problem string) error {
	tx, err := w.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var saved string
	if err = tx.QueryRowContext(ctx, "SELECT payload FROM research_checkpoints WHERE case_id=?", j.Ref).Scan(&saved); err != nil {
		return err
	}
	var checkpoint researchCheckpoint
	if err = json.Unmarshal([]byte(saved), &checkpoint); err != nil {
		return err
	}
	checkpoint.ReviewCoverage = true
	checkpoint.ReviewProblem = problem
	if problem != "" {
		if checkpoint.RepairAttempts >= 1 {
			return errors.New("the result correction attempt has already been used")
		}
		checkpoint.RepairAttempts++
	}
	if len(content) > 0 && checkpoint.Result.Outcome == "new_event" {
		parts := frontmatter.Split(string(content))
		if len(parts) != 3 {
			return errors.New("invalid draft front matter")
		}
		var metadata struct {
			AddedAt string `yaml:"added_at"`
			Code    string `yaml:"code"`
		}
		if err = yaml.Unmarshal([]byte(parts[1]), &metadata); err != nil {
			return err
		}
		checkpoint.AddedAt = metadata.AddedAt
		checkpoint.Code = metadata.Code
	}
	if j.Kind != "publish" && j.Kind != "research" {
		return errors.New("invalid coverage review job")
	}
	deleted, err := tx.ExecContext(ctx, "DELETE FROM jobs WHERE id=? AND kind=? AND status='running' AND lease_token=?", j.ID, j.Kind, j.Token)
	if err != nil {
		return err
	}
	if n, _ := deleted.RowsAffected(); n != 1 {
		return errors.New("publication lease lost before coverage review")
	}
	now := time.Now().Unix()
	if _, err = tx.ExecContext(ctx, "UPDATE research_checkpoints SET payload=? WHERE case_id=?", store.JSON(checkpoint), j.Ref); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,kind,ref,payload,status,available_at) VALUES(?,'research',?,'{}','queued',?)
 ON CONFLICT(kind,ref) DO UPDATE SET status='queued',available_at=excluded.available_at,attempts=0,lease_until=0,lease_token='',error_code=''`, store.ID(), j.Ref, now); err != nil {
		return err
	}
	why := "Another submission changed report coverage while this one was researching. Saved research will be checked against the latest reports before publication."
	if problem != "" {
		why = "The proposed result needs a correction before it can be validated. One research review will use the saved sources and current reports; all evidence and report checks still apply."
	}
	if _, err = tx.ExecContext(ctx, "UPDATE cases SET status='queued',explanation=?,report_slug='',draft_path='',draft_markdown='',updated_at=? WHERE id=?", why, now, j.Ref); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO submission_history(submission_id,status,explanation,actor,created_at) SELECT id,'queued',?,'worker',? FROM submissions WHERE case_id=?", why, now, j.Ref); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return errCoverageRequeued
}
