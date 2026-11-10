package api

import (
	"net/http"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// Publication is where committed reports stand: waiting for the next batch to main, on main and
// waiting for the site's build, and the last batch.
type Publication struct {
	Waiting     int64      `json:"waiting"`   // committed, not on main yet
	Deploying   int64      `json:"deploying"` // on main, not on the site yet
	Batched     bool       `json:"batched"`   // reports go to main in batches (PUBLISH_BRANCH)
	Branch      string     `json:"branch,omitempty"`
	NextBatchAt int64      `json:"next_batch_at,omitempty"`
	IntervalSec int64      `json:"interval_seconds,omitempty"`
	LastBatch   *LastBatch `json:"last_batch,omitempty"`
	// the site's latest Pages build on GitHub, while reports wait for the site
	DeployRun *store.DeployRun `json:"deploy_run,omitempty"`
}
type LastBatch struct {
	MergedAt   int64  `json:"merged_at"`
	Commit     string `json:"commit"`
	Reports    int64  `json:"reports"`
	DeployedAt int64  `json:"deployed_at,omitempty"`
}

// status is the processing indicator: the service answers, whether AI processing is on, how many
// requests are being worked on or waiting, and where committed reports stand. A few small
// queries, cheap enough to poll.
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	var running, queued int64
	pub := Publication{Batched: s.Config.PublishBranch != "", Branch: s.Config.PublishBranch}
	err := s.Store.Reader().QueryRowContext(r.Context(), `SELECT
 coalesce(sum(status IN ('fetching','classifying','researching','verifying','drafted','awaiting_review','publishing')),0),
 coalesce(sum(status IN ('queued','budget_wait')),0),
 coalesce(sum(status='committed'),0),
 coalesce(sum(status IN ('merged','deploying')),0) FROM cases`).Scan(&running, &queued, &pub.Waiting, &pub.Deploying)
	if err != nil {
		problem(w, 503, "Status unavailable.")
		return
	}
	if pub.Batched {
		pub.NextBatchAt = s.Config.NextBatch(time.Now()).Unix()
		pub.IntervalSec = int64(s.Config.PublishInterval.Seconds())
	}
	var last LastBatch
	if s.Store.Reader().QueryRowContext(r.Context(), "SELECT merged_at,commit_sha,reports,deployed_at FROM publication_batches ORDER BY id DESC LIMIT 1").Scan(&last.MergedAt, &last.Commit, &last.Reports, &last.DeployedAt) == nil {
		pub.LastBatch = &last
	}
	if pub.Deploying > 0 || pub.Waiting > 0 {
		pub.DeployRun = s.Store.DeployRun()
	}
	p := s.processingInfo(r)
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, struct {
		Online      bool        `json:"online"`
		Processing  bool        `json:"processing"`
		Reason      string      `json:"reason,omitempty"`
		Running     int64       `json:"running"`
		Queued      int64       `json:"queued"`
		Publication Publication `json:"publication"`
	}{true, p.Enabled, p.Reason, running, queued, pub})
}
