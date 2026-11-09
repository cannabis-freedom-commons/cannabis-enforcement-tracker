package worker

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// API is the backend's side of the worker API (docs/WORKERS.md): separate workers take research
// jobs here and do every database step of them through it. A call is accepted only for the job
// whose lease it names, and only for that job's case.
type API struct {
	Worker *Worker // the backend's own worker: its store, configuration and job completion
	Health *research.Health

	mu      sync.Mutex
	workers map[string]workerSeen
}

type workerSeen struct {
	at time.Time
	presence
}

const workerLease = 2 * time.Minute

// offlineAfter: a worker not heard from for this long counts as gone.
const offlineAfter = 90 * time.Second

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+apiPrefix+"hello", a.authorized(a.hello))
	mux.HandleFunc("POST "+apiPrefix+"claim", a.authorized(a.claim))
	mux.HandleFunc("POST "+apiPrefix+"heartbeat", a.authorized(a.heartbeat))
	mux.HandleFunc("POST "+apiPrefix+"finish", a.authorized(a.finish))
	mux.HandleFunc("POST "+apiPrefix+"call", a.authorized(a.call))
	mux.HandleFunc("POST "+apiPrefix+"upload/{kind}", a.authorized(a.upload))
	return mux
}

func (a *API) store() *store.Store   { return a.Worker.Store }
func (a *API) config() config.Config { return a.Worker.Config }

type workerName struct{}

func (a *API) authorized(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		name := ""
		if ok {
			for known, n := range a.config().WorkerTokens {
				if subtle.ConstantTimeCompare([]byte(known), []byte(token)) == 1 {
					name = n
				}
			}
		}
		if name == "" {
			http.Error(w, "unknown worker token", http.StatusUnauthorized)
			return
		}
		next(w, r, name)
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(v); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// fail answers a call that went wrong: 409 for a lost lease (the worker stops the job), else 422
// with the error, so the worker's job sees the same error it would have seen in one process.
func fail(w http.ResponseWriter, err error) {
	status := http.StatusUnprocessableEntity
	if errors.Is(err, errLeaseLost) {
		status = http.StatusConflict
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(encodeError(err))
}

// seen notes a worker and what it can do; the status page shows the research of the workers
// heard from recently.
func (a *API) seen(name string, p presence) {
	a.mu.Lock()
	if a.workers == nil {
		a.workers = map[string]workerSeen{}
	}
	a.workers[name] = workerSeen{time.Now(), p}
	a.mu.Unlock()
	a.updateHealth()
}

// Agents are the research agents of the workers heard from recently, with the runner they use.
func (a *API) Agents() ([]config.ResearchAgent, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var agents []config.ResearchAgent
	runners := map[string]bool{}
	names := make([]string, 0, len(a.workers))
	for name := range a.workers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		seen := a.workers[name]
		if time.Since(seen.at) > offlineAfter {
			continue
		}
		for _, agent := range seen.Agents {
			agents = append(agents, config.ResearchAgent{Model: agent.Model, MaxWorkers: agent.MaxWorkers})
		}
		if seen.Runner != "" {
			runners[seen.Runner] = true
		}
	}
	list := []string{}
	for r := range runners {
		list = append(list, r)
	}
	sort.Strings(list)
	return agents, strings.Join(list, ", ")
}

// updateHealth sets the backend's research health from its workers: ready when one is.
func (a *API) updateHealth() {
	if a.Health == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	state := research.HealthSnapshot{Status: "failed", Message: "No research worker is connected. Requests wait in the queue until one is.", CheckedAt: time.Now().Unix()}
	var names []string
	for name, seen := range a.workers {
		if time.Since(seen.at) > offlineAfter {
			continue
		}
		names = append(names, name)
		if seen.Health != nil && seen.Health.Status == "ready" {
			state = *seen.Health
		} else if state.Status != "ready" && seen.Health != nil {
			state = *seen.Health
		}
	}
	sort.Strings(names)
	if len(names) > 0 {
		state.Message = strings.TrimSpace(state.Message + " Research workers: " + strings.Join(names, ", ") + ".")
	}
	before := a.Health.Snapshot()
	a.Health.Set(state)
	if before.Status != state.Status {
		a.store().FeedChanged(context.Background())
	}
}

// Watch marks workers offline when they stop calling in, until ctx ends.
func (a *API) Watch(ctx context.Context) {
	a.updateHealth()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.updateHealth()
		}
	}
}

// hello is a worker saying it is there, and how its research agents are, while it takes no job.
func (a *API) hello(w http.ResponseWriter, r *http.Request, name string) {
	var req claimRequest
	if !readJSON(w, r, &req) {
		return
	}
	a.seen(name, req.presence)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) claim(w http.ResponseWriter, r *http.Request, name string) {
	var req claimRequest
	if !readJSON(w, r, &req) {
		return
	}
	a.seen(name, req.presence)
	j, err := a.store().ClaimKind(r.Context(), "jobs", "research", workerLease)
	if errors.Is(err, sql.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		slog.Error("worker claim failed", "worker", name, "error", err)
		http.Error(w, "claim failed", http.StatusInternalServerError)
		return
	}
	slog.Info("research job leased to worker", "worker", name, "job", j.ID, "case", j.Ref, "attempt", j.Attempts)
	writeJSON(w, toWire(j))
}

func (a *API) heartbeat(w http.ResponseWriter, r *http.Request, name string) {
	var req heartbeatRequest
	if !readJSON(w, r, &req) {
		return
	}
	a.seen(name, req.presence)
	if err := a.store().Heartbeat(r.Context(), "jobs", req.Job.job(), workerLease); err != nil {
		fail(w, errLeaseLost)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) finish(w http.ResponseWriter, r *http.Request, name string) {
	var req finishRequest
	if !readJSON(w, r, &req) {
		return
	}
	j, err := a.lease(r.Context(), req.Job)
	if err != nil {
		fail(w, err)
		return
	}
	a.Worker.finishJob(r.Context(), j, decodeError(req.Error), true)
	w.WriteHeader(http.StatusNoContent)
}

// lease checks that a job is still leased with the token the worker holds, and returns it as the
// backend knows it.
func (a *API) lease(ctx context.Context, wj wireJob) (store.Job, error) {
	j := store.Job{ID: wj.ID, Token: wj.Token}
	if wj.ID == "" || wj.Token == "" {
		return j, errLeaseLost
	}
	err := a.store().DB.QueryRowContext(ctx, "SELECT kind,ref,payload,attempts FROM jobs WHERE id=? AND lease_token=? AND status='running' AND kind='research'", wj.ID, wj.Token).Scan(&j.Kind, &j.Ref, &j.Payload, &j.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return j, errLeaseLost
	}
	return j, err
}

// statuses a worker may not set: publication is the backend's.
var backendStatuses = map[string]bool{"publishing": true, "committed": true, "published": true, "merged": true, "deploying": true}

var workspaceDir = regexp.MustCompile(`^[0-9a-f]{32}-[0-9]+$`)

type args map[string]any

func (a args) str(k string) string {
	v, _ := a[k].(string)
	return v
}
func (a args) num(k string) int64 {
	v, _ := a[k].(float64)
	return int64(v)
}

// decode reads one argument into v by way of JSON.
func (a args) decode(k string, v any) error {
	b, err := json.Marshal(a[k])
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (a *API) call(w http.ResponseWriter, r *http.Request, name string) {
	var req callRequest
	if !readJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	j, err := a.lease(ctx, req.Job)
	if err != nil {
		fail(w, err)
		return
	}
	result, err := a.op(ctx, j, req.Op, args(req.Args))
	if err != nil {
		if !errors.Is(err, errCoverageRequeued) && !errors.Is(err, errHeldBack) && !errors.Is(err, store.ErrBudget) && !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("worker call failed", "worker", name, "op", req.Op, "case", j.Ref, "error", err)
		}
		fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"result": result})
}

// op runs one Backend operation for job j.
func (a *API) op(ctx context.Context, j store.Job, op string, in args) (any, error) {
	db := Local{a.store()}
	cid := in.str("cid")
	if _, ok := in["cid"]; ok && cid != j.Ref {
		return nil, errors.New("the call names another case than its job")
	}
	run := in.str("run")
	if run != "" {
		var owner string
		if err := a.store().DB.QueryRowContext(ctx, "SELECT case_id FROM research_runs WHERE id=?", run).Scan(&owner); err != nil || owner != j.Ref {
			return nil, errors.New("the call names a research run of another case")
		}
	}
	switch op {
	case "start_run":
		return nil, db.StartRun(ctx, run, in.str("model"), in.str("runner"))
	case "end_run":
		return nil, db.EndRun(ctx, run, in.str("status"), in.str("outcome"))
	case "ai_event":
		return nil, db.AddAIEvent(ctx, cid, run, in.str("role"), in.str("kind"), in.str("text"))
	case "update_run":
		var u store.RunUpdate
		if err := in.decode("update", &u); err != nil {
			return nil, err
		}
		if u.Workspace != nil {
			// the run's logs are kept in the backend's mirror of the worker's workspace
			dir, err := a.mirror(j.Ref, *u.Workspace)
			if err != nil {
				return nil, err
			}
			u.Workspace = &dir
		}
		return nil, db.UpdateRun(ctx, run, u)
	case "feed_changed":
		db.FeedChanged(ctx)
		return nil, nil
	case "case_source":
		u, status, queued, err := db.CaseSource(ctx, cid)
		return map[string]any{"url": u, "status": status, "publish_queued": queued}, err
	case "transition":
		status := in.str("status")
		if backendStatuses[status] {
			return nil, fmt.Errorf("a worker cannot set the status %s", status)
		}
		return nil, db.Transition(ctx, cid, status, in.str("explanation"), in.str("slug"), in.str("pr"))
	case "set_case_output":
		var values map[string]string
		if err := in.decode("values", &values); err != nil {
			return nil, err
		}
		return nil, db.SetCaseOutput(ctx, cid, values)
	case "first_submission":
		return db.FirstSubmission(ctx, cid)
	case "checkpoint":
		return db.Checkpoint(ctx, cid)
	case "save_checkpoint":
		return nil, db.SaveCheckpoint(ctx, cid, in.str("payload"))
	case "reserve":
		// the budget is the backend's, whatever the worker is configured with
		return db.Reserve(ctx, cid, a.config().RunCents, a.config().DailyCents)
	case "claim_source":
		return db.ClaimSource(ctx, cid, in.str("url"), in.str("final_url"), in.str("digest"))
	case "reject_source_overlap":
		var check research.Validation
		if err := in.decode("check", &check); err != nil {
			return nil, err
		}
		return nil, db.RejectSourceOverlap(ctx, cid, in.str("other"), in.str("source"), check)
	case "recent_source_checks":
		return db.RecentSourceChecks(ctx)
	case "researching_source_checks":
		return db.ResearchingSourceChecks(ctx, cid)
	case "hold_back":
		return db.HoldBack(ctx, j, in.str("other"))
	case "requeue_review":
		return nil, db.RequeueReview(ctx, j, []byte(in.str("content")), in.str("problem"))
	}
	return nil, fmt.Errorf("unknown operation %q", op)
}

// mirror is the backend's folder for a worker workspace of case cid: the same name, under the
// backend's job storage, so its logs and drafts are cleaned up like local ones.
func (a *API) mirror(cid, workerDir string) (string, error) {
	base := filepath.Base(workerDir)
	if !workspaceDir.MatchString(base) || !strings.HasPrefix(base, cid+"-") {
		return "", errors.New("invalid workspace name")
	}
	root, err := filepath.Abs(a.config().Workspaces)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, base)
	return dir, os.MkdirAll(dir, 0700)
}

// upload takes the files of a job: a source capture, a research run's logs, or a validated draft.
func (a *API) upload(w http.ResponseWriter, r *http.Request, name string) {
	// a draft with its photos can take a while over a home connection
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(10 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(11 * time.Minute))
	q := r.URL.Query()
	ctx := r.Context()
	j, err := a.lease(ctx, wireJob{ID: q.Get("job"), Token: q.Get("token")})
	if err != nil {
		fail(w, err)
		return
	}
	body := http.MaxBytesReader(w, r.Body, maxArchiveBytes+1<<20)
	switch r.PathValue("kind") {
	case "capture":
		err = a.uploadCapture(ctx, j, q.Get("capture"), q.Get("url"), body)
	case "runlog":
		err = a.uploadRunLog(ctx, j, q.Get("run"), body)
	case "draft":
		err = a.uploadDraft(ctx, j, q.Get("workspace"), body)
	default:
		err = errors.New("unknown upload")
	}
	if err != nil {
		slog.Warn("worker upload failed", "worker", name, "kind", r.PathValue("kind"), "case", j.Ref, "error", err)
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var captureID = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (a *API) uploadCapture(ctx context.Context, j store.Job, capture, url string, body io.Reader) error {
	if !captureID.MatchString(capture) {
		return errors.New("invalid capture")
	}
	if _, err := safeurl.Normalize(url); err != nil {
		return err
	}
	root, err := filepath.Abs(a.config().Workspaces)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "sources", j.Ref, capture)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if _, err = readArchive(body, dir, nil); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	return Local{a.store()}.RecordCapture(ctx, j.Ref, safeurl.Evidence{CaptureID: capture, URL: url, ArtifactDir: dir})
}

// the logs the request page's raw view reads (api/ai_raw.go)
var runLogs = map[string]bool{"research-events.jsonl": true, "research-stderr.log": true}

func (a *API) uploadRunLog(ctx context.Context, j store.Job, run string, body io.Reader) error {
	var owner, workspace string
	if err := a.store().DB.QueryRowContext(ctx, "SELECT case_id,workspace FROM research_runs WHERE id=?", run).Scan(&owner, &workspace); err != nil || owner != j.Ref {
		return errors.New("the upload names a research run of another case")
	}
	if workspace == "" {
		return errors.New("the run has no workspace")
	}
	dir, err := a.mirror(j.Ref, workspace)
	if err != nil {
		return err
	}
	_, err = readArchive(body, dir, func(rel string) bool { return runLogs[rel] })
	return err
}

// draftManifest is the description of an uploaded draft, at the top of its archive.
const draftManifest = "draft.json"

func (a *API) uploadDraft(ctx context.Context, j store.Job, workspace string, body io.Reader) error {
	dir, err := a.mirror(j.Ref, workspace)
	if err != nil {
		return err
	}
	staging, err := os.MkdirTemp(dir, ".upload-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if _, err = readArchive(body, staging, func(rel string) bool {
		return rel == draftManifest || reportPath.MatchString(rel) || strings.HasPrefix(rel, "media/") && !strings.Contains(strings.TrimPrefix(rel, "media/"), "/")
	}); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(staging, draftManifest))
	if err != nil {
		return err
	}
	var d draftJob
	if err = json.Unmarshal(b, &d); err != nil {
		return err
	}
	if !reportPath.MatchString(d.Rel) || (d.PreviousRel != "" && !reportPath.MatchString(d.PreviousRel)) || (d.MediaDir != "" && d.MediaDir != "media") {
		return errors.New("invalid draft paths")
	}
	files := map[string]string{d.Rel: d.Digest}
	for _, u := range d.Updates {
		if !reportPath.MatchString(u.Rel) {
			return errors.New("invalid report update path")
		}
		files[u.Rel] = u.Digest
	}
	for rel, digest := range files {
		data, err := os.ReadFile(filepath.Join(staging, rel))
		if err != nil {
			return err
		}
		if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != digest {
			return fmt.Errorf("uploaded %s does not match its digest", rel)
		}
	}
	// the draft replaces whatever an earlier attempt left in the mirror
	for _, rel := range append(sortedKeys(files), "media") {
		target := filepath.Join(dir, rel)
		_ = os.RemoveAll(target)
		if _, err := os.Stat(filepath.Join(staging, rel)); os.IsNotExist(err) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(staging, rel), target); err != nil {
			return err
		}
	}
	d.Dir, d.Uploaded = dir, true
	return Local{a.store()}.QueuePublish(ctx, j, d)
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
