package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// Client is a worker's connection to the backend's worker API.
type Client struct {
	URL, Token string
	Repo       string // the worker's clone of the reports repository
	HTTP       *http.Client
	// Presence says what this worker can do; sent with every claim and heartbeat.
	Presence func() presence
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// retryFor: how long a call keeps trying while the backend cannot be reached (a restart, a
// network drop at home) before the job gives up its attempt.
const retryFor = 3 * time.Minute

// do sends one request, retrying while the backend is unreachable or answers 5xx. A refused call
// (409, 422) comes back as its error; 401 and other 4xx as a plain error.
func (c *Client) do(ctx context.Context, path, contentType string, body func() io.Reader, out any) (int, error) {
	deadline := time.Now().Add(retryFor)
	delay := time.Second
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+path, body())
		if err != nil {
			return 0, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Content-Type", contentType)
		resp, err := c.http().Do(req)
		if err == nil {
			status := resp.StatusCode
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
			resp.Body.Close()
			switch {
			case readErr != nil:
				err = readErr
			case status == http.StatusNoContent:
				return status, nil
			case status == http.StatusOK:
				if out != nil {
					return status, json.Unmarshal(data, out)
				}
				return status, nil
			case status == http.StatusConflict || status == http.StatusUnprocessableEntity:
				var we wireError
				if json.Unmarshal(data, &we) != nil {
					we.Message = strings.TrimSpace(string(data))
				}
				return status, decodeError(&we)
			case status < 500:
				return status, fmt.Errorf("backend refused %s: %d %s", path, status, strings.TrimSpace(string(data)))
			default:
				err = fmt.Errorf("backend answered %d", status)
			}
		}
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("backend unreachable: %w", err)
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(delay):
		}
		delay = min(2*delay, 20*time.Second)
	}
}

func (c *Client) post(ctx context.Context, path string, in, out any) (int, error) {
	b, err := json.Marshal(in)
	if err != nil {
		return 0, err
	}
	return c.do(ctx, path, "application/json", func() io.Reader { return bytes.NewReader(b) }, out)
}

func (c *Client) presence() presence {
	if c.Presence == nil {
		return presence{}
	}
	return c.Presence()
}

// Claim takes the next research job; ok is false when there is none.
func (c *Client) Claim(ctx context.Context) (store.Job, bool, error) {
	var j wireJob
	status, err := c.post(ctx, apiPrefix+"claim", claimRequest{c.presence()}, &j)
	if err != nil || status == http.StatusNoContent {
		return store.Job{}, false, err
	}
	return j.job(), true, nil
}

// Hello tells the backend this worker is there and how its agents are.
func (c *Client) Hello(ctx context.Context) error {
	_, err := c.post(ctx, apiPrefix+"hello", claimRequest{c.presence()}, nil)
	return err
}

func (c *Client) Heartbeat(ctx context.Context, j store.Job) error {
	_, err := c.post(ctx, apiPrefix+"heartbeat", heartbeatRequest{toWire(j), c.presence()}, nil)
	return err
}

func (c *Client) Finish(ctx context.Context, j store.Job, workErr error) error {
	_, err := c.post(ctx, apiPrefix+"finish", finishRequest{toWire(j), encodeError(workErr)}, nil)
	return err
}

// Job is the Backend of one leased job: every call carries the job's lease.
func (c *Client) Job(j store.Job) Backend { return &remoteJob{c: c, job: j} }

type remoteJob struct {
	c   *Client
	job store.Job
	// the local workspace of each run, whose logs are uploaded when the run ends
	runDirs sync.Map
}

func (r *remoteJob) call(ctx context.Context, op string, in map[string]any, out any) error {
	var resp struct {
		Result json.RawMessage `json:"result"`
	}
	if _, err := r.c.post(ctx, apiPrefix+"call", callRequest{Job: toWire(r.job), Op: op, Args: in}, &resp); err != nil {
		return err
	}
	if out == nil || len(resp.Result) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Result, out)
}

func (r *remoteJob) upload(ctx context.Context, kind string, query url.Values, dir string, keep func(string) bool) error {
	var buf bytes.Buffer
	if err := writeArchive(&buf, dir, keep); err != nil {
		return err
	}
	query.Set("job", r.job.ID)
	query.Set("token", r.job.Token)
	data := buf.Bytes()
	_, err := r.c.do(ctx, apiPrefix+"upload/"+kind+"?"+query.Encode(), "application/gzip", func() io.Reader { return bytes.NewReader(data) }, nil)
	return err
}

func (r *remoteJob) StartRun(ctx context.Context, id, model, runner string) error {
	return r.call(ctx, "start_run", map[string]any{"run": id, "model": model, "runner": runner}, nil)
}

func (r *remoteJob) EndRun(ctx context.Context, id, status, outcome string) error {
	if dir, ok := r.runDirs.Load(id); ok {
		if err := r.upload(ctx, "runlog", url.Values{"run": {id}}, dir.(string), func(rel string) bool { return runLogs[rel] }); err != nil {
			slog.Warn("research run log upload failed", "run", id, "error", err)
		}
	}
	return r.call(ctx, "end_run", map[string]any{"run": id, "status": status, "outcome": outcome}, nil)
}

func (r *remoteJob) AddAIEvent(ctx context.Context, cid, run, role, kind, text string) error {
	return r.call(ctx, "ai_event", map[string]any{"cid": cid, "run": run, "role": role, "kind": kind, "text": text}, nil)
}

func (r *remoteJob) UpdateRun(ctx context.Context, id string, u store.RunUpdate) error {
	if u.Workspace != nil {
		r.runDirs.Store(id, *u.Workspace)
	}
	return r.call(ctx, "update_run", map[string]any{"run": id, "update": u}, nil)
}

func (r *remoteJob) FeedChanged(ctx context.Context) { _ = r.call(ctx, "feed_changed", nil, nil) }

func (r *remoteJob) CaseSource(ctx context.Context, cid string) (string, string, bool, error) {
	var out struct {
		URL           string `json:"url"`
		Status        string `json:"status"`
		PublishQueued bool   `json:"publish_queued"`
	}
	err := r.call(ctx, "case_source", map[string]any{"cid": cid}, &out)
	return out.URL, out.Status, out.PublishQueued, err
}

func (r *remoteJob) Transition(ctx context.Context, cid, status, explanation, slug, pr string) error {
	return r.call(ctx, "transition", map[string]any{"cid": cid, "status": status, "explanation": explanation, "slug": slug, "pr": pr}, nil)
}

func (r *remoteJob) SetCaseOutput(ctx context.Context, cid string, values map[string]string) error {
	return r.call(ctx, "set_case_output", map[string]any{"cid": cid, "values": values}, nil)
}

func (r *remoteJob) FirstSubmission(ctx context.Context, cid string) (string, error) {
	var id string
	err := r.call(ctx, "first_submission", map[string]any{"cid": cid}, &id)
	return id, err
}

func (r *remoteJob) Checkpoint(ctx context.Context, cid string) (string, error) {
	var payload string
	err := r.call(ctx, "checkpoint", map[string]any{"cid": cid}, &payload)
	return payload, err
}

func (r *remoteJob) SaveCheckpoint(ctx context.Context, cid, payload string) error {
	return r.call(ctx, "save_checkpoint", map[string]any{"cid": cid, "payload": payload}, nil)
}

func (r *remoteJob) Reserve(ctx context.Context, cid string, _, _ int) (string, error) {
	var run string
	err := r.call(ctx, "reserve", map[string]any{"cid": cid}, &run)
	return run, err
}

func (r *remoteJob) RecordCapture(ctx context.Context, cid string, e safeurl.Evidence) error {
	return r.upload(ctx, "capture", url.Values{"capture": {e.CaptureID}, "url": {e.URL}}, e.ArtifactDir, nil)
}

func (r *remoteJob) ClaimSource(ctx context.Context, cid, u, finalURL, digest string) (string, error) {
	var owner string
	err := r.call(ctx, "claim_source", map[string]any{"cid": cid, "url": u, "final_url": finalURL, "digest": digest}, &owner)
	return owner, err
}

func (r *remoteJob) RejectSourceOverlap(ctx context.Context, cid, other, source string, check research.Validation) error {
	return r.call(ctx, "reject_source_overlap", map[string]any{"cid": cid, "other": other, "source": source, "check": check}, nil)
}

func (r *remoteJob) RecentSourceChecks(ctx context.Context) ([]string, error) {
	var out []string
	err := r.call(ctx, "recent_source_checks", nil, &out)
	return out, err
}

func (r *remoteJob) ResearchingSourceChecks(ctx context.Context, cid string) (map[string]string, error) {
	var out map[string]string
	err := r.call(ctx, "researching_source_checks", map[string]any{"cid": cid}, &out)
	return out, err
}

func (r *remoteJob) HoldBack(ctx context.Context, _ store.Job, other string) (bool, error) {
	var held bool
	err := r.call(ctx, "hold_back", map[string]any{"other": other}, &held)
	return held, err
}

func (r *remoteJob) RequeueReview(ctx context.Context, _ store.Job, content []byte, problem string) error {
	return r.call(ctx, "requeue_review", map[string]any{"content": string(content), "problem": problem}, nil)
}

// QueuePublish uploads the validated draft, the git blobs its changed reports started from, and
// its new photos (downloaded here, so the backend opens no page); the backend queues its publication.
func (r *remoteJob) QueuePublish(ctx context.Context, _ store.Job, d draftJob) error {
	repo := r.c.Repo
	pack, err := os.MkdirTemp("", "tracker-draft-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(pack)
	rels := []string{d.Rel}
	for _, u := range d.Updates {
		rels = append(rels, u.Rel)
	}
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(d.Dir, rel))
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Join(pack, filepath.Dir(rel)), 0700); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(pack, rel), data, 0600); err != nil {
			return err
		}
	}
	bases := []string{}
	if d.PreviousRel != "" {
		bases = append(bases, d.PreviousRel)
	}
	for _, u := range d.Updates {
		bases = append(bases, u.Rel)
	}
	d.BaseBlobs = map[string]string{}
	for _, path := range bases {
		cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD:"+path)
		cmd.Dir = d.Dir
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("base of %s: %w", path, err)
		}
		d.BaseBlobs[path] = strings.TrimSpace(string(out))
	}
	media, mediaErr := prepareMedia(ctx, repo, d.Dir, rels)
	defer media.Close()
	if mediaErr != nil {
		d.MediaError = mediaErr.Error()
	} else if media != nil {
		if err = media.pack(filepath.Join(pack, "media")); err != nil {
			return err
		}
		d.MediaDir = "media"
	}
	workspace := filepath.Base(d.Dir)
	d.Dir, d.Uploaded = "", true
	if err = os.WriteFile(filepath.Join(pack, draftManifest), []byte(store.JSON(d)), 0600); err != nil {
		return err
	}
	return r.upload(ctx, "draft", url.Values{"workspace": {workspace}}, pack, nil)
}
