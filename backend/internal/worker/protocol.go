package worker

// The worker API between the backend and separate research workers (docs/WORKERS.md): the
// messages both sides exchange, how errors travel, and the archives files travel in.

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

const apiPrefix = "/worker/v1/"

// wireJob is a leased job as the worker holds it. The token proves the lease on every call.
type wireJob struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Ref      string `json:"ref"`
	Payload  string `json:"payload"`
	Token    string `json:"token"`
	Attempts int    `json:"attempts"`
}

func toWire(j store.Job) wireJob {
	return wireJob{ID: j.ID, Kind: j.Kind, Ref: j.Ref, Payload: j.Payload, Token: j.Token, Attempts: j.Attempts}
}
func (j wireJob) job() store.Job {
	return store.Job{ID: j.ID, Kind: j.Kind, Ref: j.Ref, Payload: j.Payload, Token: j.Token, Attempts: j.Attempts}
}

// presence is what a worker says about itself with every claim and heartbeat.
type presence struct {
	Agents []agentInfo              `json:"agents"`
	Runner string                   `json:"runner"`
	Health *research.HealthSnapshot `json:"health,omitempty"`
	Stats  *workerStats             `json:"stats,omitempty"`
}

// workerStats are a worker's own figures since it started, for the backend's /metrics.
type workerStats struct {
	StartedAt  int64            `json:"started_at"`
	Running    int              `json:"running"`
	Jobs       map[string]int64 `json:"jobs"`        // finished jobs by result: done, failed, interrupted, requeued, lost
	JobSeconds float64          `json:"job_seconds"` // time spent on them
}
type agentInfo struct {
	Model      string `json:"model"`
	MaxWorkers int    `json:"max_workers"`
}

type claimRequest struct {
	presence
}
type heartbeatRequest struct {
	Job wireJob `json:"job"`
	presence
}
type finishRequest struct {
	Job   wireJob    `json:"job"`
	Error *wireError `json:"error,omitempty"`
}
type callRequest struct {
	Job  wireJob        `json:"job"`
	Op   string         `json:"op"`
	Args map[string]any `json:"args"`
}

// wireError carries an error and, for the errors the job loop treats specially, which one it is.
type wireError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

var errLeaseLost = errors.New("the job's lease was lost")

var errorCodes = []struct {
	code string
	err  error
}{
	{"budget", store.ErrBudget},
	{"configuration", store.ErrConfiguration},
	{"interrupted", store.ErrInterrupted},
	{"research_agent", store.ErrResearchAgent},
	{"requeued", errCoverageRequeued},
	{"held_back", errHeldBack},
	{"no_rows", sql.ErrNoRows},
	{"lease_lost", errLeaseLost},
}

func encodeError(err error) *wireError {
	if err == nil {
		return nil
	}
	for _, e := range errorCodes {
		if errors.Is(err, e.err) {
			return &wireError{Code: e.code, Message: err.Error()}
		}
	}
	return &wireError{Message: err.Error()}
}

// remoteError is an error that happened on the other side; errors.Is still finds its kind.
type remoteError struct {
	message string
	kind    error
}

func (e *remoteError) Error() string { return e.message }
func (e *remoteError) Unwrap() error { return e.kind }

func decodeError(w *wireError) error {
	if w == nil {
		return nil
	}
	for _, e := range errorCodes {
		if e.code == w.Code {
			return &remoteError{w.Message, e.err}
		}
	}
	return errors.New(w.Message)
}

// Archive limits: a capture holds a screenshot and the page's HTML, a draft its reports and photos.
const (
	maxArchiveBytes = 256 << 20
	maxArchiveFiles = 2000
)

// writeArchive writes the regular files under dir (paths relative to it, filtered by keep when
// set) as a gzipped tar. Symlinks and anything else are left out.
func writeArchive(w io.Writer, dir string, keep func(rel string) bool) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if keep != nil && !keep(rel) {
			return nil
		}
		return addFile(tw, rel, path)
	})
	if err != nil {
		return err
	}
	if err = tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func addFile(tw *tar.Writer, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if err = tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: info.Size(), ModTime: info.ModTime(), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

// readArchive unpacks a gzipped tar into dir: regular files only, at relative paths inside it,
// within the size limits. keep, when set, says which paths are wanted; others are refused.
func readArchive(r io.Reader, dir string, keep func(rel string) bool) ([]string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	var names []string
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return names, nil
		}
		if err != nil {
			return names, err
		}
		if h.Typeflag != tar.TypeReg {
			return names, fmt.Errorf("archive entry %q is not a regular file", h.Name)
		}
		name := filepath.ToSlash(filepath.Clean(h.Name))
		if name == "." || strings.HasPrefix(name, "../") || name == ".." || filepath.IsAbs(name) || strings.Contains(name, "\\") {
			return names, fmt.Errorf("archive entry %q is outside its folder", h.Name)
		}
		if keep != nil && !keep(name) {
			return names, fmt.Errorf("archive entry %q is not expected", h.Name)
		}
		total += h.Size
		if len(names) >= maxArchiveFiles || total > maxArchiveBytes {
			return names, errors.New("archive is too large")
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return names, err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return names, err
		}
		_, err = io.Copy(f, io.LimitReader(tr, h.Size))
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return names, err
		}
		names = append(names, name)
	}
}
