package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// Backend is everything a research job reads from or writes to the database. In one process it is
// the store (Local); on a separate worker it is the backend's worker API (see docs/WORKERS.md).
// Calls that change a job's queue entry take the job, so a lost lease is refused.
type Backend interface {
	research.RunLog

	// CaseSource: the submitted URL and status of a case, and whether its publication is queued.
	CaseSource(ctx context.Context, cid string) (url, status string, publishQueued bool, err error)
	Transition(ctx context.Context, cid, status, explanation, slug, pr string) error
	SetCaseOutput(ctx context.Context, cid string, values map[string]string) error
	// FirstSubmission: the receipt of the case's first submission.
	FirstSubmission(ctx context.Context, cid string) (string, error)

	// Checkpoint returns the saved research of a case, "" when there is none.
	Checkpoint(ctx context.Context, cid string) (string, error)
	SaveCheckpoint(ctx context.Context, cid, payload string) error
	// Reserve opens a research run within the budget (store.ErrBudget when it is spent).
	Reserve(ctx context.Context, cid string, perRun, daily int) (string, error)

	// RecordCapture keeps a rendered capture of a page (screenshot, HTML) for the request page.
	RecordCapture(ctx context.Context, cid string, e safeurl.Evidence) error
	ClaimSource(ctx context.Context, cid, url, finalURL, digest string) (string, error)
	RejectSourceOverlap(ctx context.Context, cid, other, source string, check research.Validation) error

	// RecentSourceChecks: the source checks of the last three days' cases.
	RecentSourceChecks(ctx context.Context) ([]string, error)
	// ResearchingSourceChecks: the source checks of other cases being researched now, by case.
	ResearchingSourceChecks(ctx context.Context, cid string) (map[string]string, error)
	HoldBack(ctx context.Context, j store.Job, other string) (bool, error)

	// RequeueReview replaces the job with a research review (errCoverageRequeued on success).
	RequeueReview(ctx context.Context, j store.Job, content []byte, problem string) error
	// QueuePublish hands a validated draft to the publish lane.
	QueuePublish(ctx context.Context, j store.Job, d draftJob) error
}

// Local is the Backend of research running inside the backend process.
type Local struct{ *store.Store }

func (b Local) UpdateRun(ctx context.Context, id string, u store.RunUpdate) error {
	return b.Store.UpdateRun(ctx, id, u)
}

func (b Local) CaseSource(ctx context.Context, cid string) (string, string, bool, error) {
	var publications int
	if err := b.DB.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE kind='publish' AND ref=?", cid).Scan(&publications); err != nil {
		return "", "", false, err
	}
	var u, status string
	if err := b.DB.QueryRowContext(ctx, "SELECT canonical_url,status FROM cases WHERE id=?", cid).Scan(&u, &status); err != nil {
		return "", "", false, err
	}
	return u, status, publications > 0, nil
}

func (b Local) FirstSubmission(ctx context.Context, cid string) (string, error) {
	var id string
	err := b.DB.QueryRowContext(ctx, "SELECT id FROM submissions WHERE case_id=? ORDER BY created_at, id LIMIT 1", cid).Scan(&id)
	return id, err
}

func (b Local) Checkpoint(ctx context.Context, cid string) (string, error) {
	var saved string
	err := b.DB.QueryRowContext(ctx, "SELECT payload FROM research_checkpoints WHERE case_id=?", cid).Scan(&saved)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return saved, err
}

func (b Local) SaveCheckpoint(ctx context.Context, cid, payload string) error {
	_, err := b.DB.ExecContext(ctx, "INSERT INTO research_checkpoints(case_id,payload,created_at) VALUES(?,?,?) ON CONFLICT(case_id) DO UPDATE SET payload=excluded.payload,created_at=excluded.created_at", cid, payload, time.Now().Unix())
	return err
}

func (b Local) Reserve(ctx context.Context, cid string, perRun, daily int) (string, error) {
	return b.Store.Reserve(ctx, cid, "", perRun, daily)
}

func (b Local) RecordCapture(ctx context.Context, cid string, e safeurl.Evidence) error {
	_, err := b.DB.ExecContext(ctx, "INSERT INTO source_captures(id,case_id,url,artifact_dir,created_at) VALUES(?,?,?,?,?)", e.CaptureID, cid, e.URL, e.ArtifactDir, time.Now().Unix())
	return err
}

// holdsSources: the case statuses that keep their sources, as they make or made a report from them.
// A case that is only queued, still researching, stopped or rejected holds none.
var holdsSources = map[string]bool{"verifying": true, "drafted": true, "publishing": true, "committed": true, "published": true}

// ClaimSource records that this case uses a source and returns the case that holds it: this one, or
// another case that holds it (holdsSources). A source recorded by a case that does not hold it
// (queued, failed, rejected) passes to this case rather than blocking it.
func (b Local) ClaimSource(ctx context.Context, cid, url, finalURL, digest string) (string, error) {
	var err error
	if url, err = safeurl.Normalize(url); err != nil {
		return "", err
	}
	if finalURL == "" {
		finalURL = url
	}
	if finalURL, err = safeurl.Normalize(finalURL); err != nil {
		return "", err
	}
	tx, err := b.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	// Distinct submitted URLs can redirect to the same article. Check both sides
	// before recording a new claim so an active/published owner cannot be bypassed.
	var aliasOwner string
	err = tx.QueryRowContext(ctx, `SELECT s.case_id FROM source_urls s JOIN cases c ON c.id=s.case_id
 WHERE (s.url IN (?,?) OR s.final_url IN (?,?)) AND s.case_id<>?
 AND c.status IN ('verifying','drafted','publishing','committed','published')
 ORDER BY s.fetched_at,s.case_id LIMIT 1`, url, finalURL, url, finalURL, cid).Scan(&aliasOwner)
	if err == nil {
		return aliasOwner, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
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

// RejectSourceOverlap is a service validation stop, not a model rejection. Keep its context
// separate from the saved model assessment, and link only public source/receipt data.
func (b Local) RejectSourceOverlap(ctx context.Context, cid, other, source string, check research.Validation) error {
	var receipt, report, status string
	if err := b.DB.QueryRowContext(ctx, `SELECT s.id,c.report_slug,c.status FROM submissions s JOIN cases c ON c.id=s.case_id WHERE c.id=? ORDER BY s.created_at,s.id LIMIT 1`, other).Scan(&receipt, &report, &status); err != nil {
		return err
	}
	var validation map[string]any
	if err := json.Unmarshal([]byte(store.JSON(check)), &validation); err != nil {
		return err
	}
	validation["source_overlap"] = map[string]string{"url": source, "submission_id": receipt, "report_slug": report, "status": status}
	if err := b.SetCaseOutput(ctx, cid, map[string]string{"validation": store.JSON(validation)}); err != nil {
		return err
	}
	reason := fmt.Sprintf("Publication stopped because an opened source is already attached to another submission, whose current status is %s. The service could not establish whether this is the same event, a later development, or separate events covered by the same article. It left the existing case intact and did not create or update a report. Compare the shared source and related submission below; a source that clearly identifies the event or its relationship to the existing case is needed to resolve the overlap.", status)
	return b.Transition(ctx, cid, uncertainStatus, reason, "", "")
}

func (b Local) RecentSourceChecks(ctx context.Context) ([]string, error) {
	rows, err := b.Reader().QueryContext(ctx, "SELECT source_check FROM cases WHERE updated_at>? AND source_check<>''", time.Now().Add(-72*time.Hour).Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}

func (b Local) ResearchingSourceChecks(ctx context.Context, cid string) (map[string]string, error) {
	rows, err := b.Reader().QueryContext(ctx, `SELECT c.id, c.source_check FROM cases c
    WHERE c.id<>? AND c.source_check<>'' AND c.status IN ('researching','verifying','drafted','publishing')
    AND EXISTS(SELECT 1 FROM jobs j WHERE j.ref=c.id AND ((j.kind='research' AND j.status='running' AND j.lease_until>?) OR j.kind='publish'))`, cid, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var other, raw string
		if err := rows.Scan(&other, &raw); err != nil {
			return nil, err
		}
		out[other] = raw
	}
	return out, rows.Err()
}

// QueuePublish queues the publish job of a validated draft whose files are in d.Dir.
func (b Local) QueuePublish(ctx context.Context, j store.Job, d draftJob) error {
	tx, err := b.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = store.Enqueue(tx, "jobs", "publish", j.Ref, store.JSON(d), time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}
