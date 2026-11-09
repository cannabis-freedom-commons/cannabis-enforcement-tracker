package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/reportcode"
)

var reportPath = regexp.MustCompile(`^reports/[0-9]{4}/[0-9]{2}/[a-z0-9]+(?:-[a-z0-9]+)*\.md$`)

var pushMu sync.Mutex

// pushRepo pushes the report repository's current branch to its upstream (origin), using the
// machine's own Git credentials. Bounded in time; the last line of Git's message says why it failed.
func pushRepo(ctx context.Context, repo string) error {
	pushMu.Lock() // one push at a time: concurrent jobs would trip over Git's ref locks
	defer pushMu.Unlock()
	pushCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(pushCtx, "git", "push", "--quiet", "origin", "HEAD")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	why := strings.TrimSpace(lines[len(lines)-1])
	if why == "" {
		why = err.Error()
	}
	if len(why) > 200 {
		why = why[:200]
	}
	return errors.New(why)
}

// Commit only the validated report paths; --only excludes unrelated staged work.
// The case trailer recovers a successful Git commit after a crash before the DB update.
func commitReport(ctx context.Context, repo, cid string, d draftJob) (string, error) {
	sha, _, err := commitReportWithMedia(ctx, repo, cid, d, nil)
	return sha, err
}

// commitReportWithMedia commits the report and, in the same commit, the manifest entries and
// thumbnails of its new photos (media, downloaded beforehand; nil for none). A thumbnail failure
// leaves the report to be committed alone; note says what became of the photos.
func commitReportWithMedia(ctx context.Context, repo, cid string, d draftJob, media *mediaBatch) (string, string, error) {
	if !reportPath.MatchString(d.Rel) || (d.PreviousRel != "" && !reportPath.MatchString(d.PreviousRel)) {
		return "", "", errors.New("invalid report path")
	}
	updates := map[string][]byte{}
	for _, u := range d.Updates {
		if !reportPath.MatchString(u.Rel) || u.Rel == d.Rel || u.Rel == d.PreviousRel || updates[u.Rel] != nil {
			return "", "", errors.New("invalid report update path")
		}
		data, err := os.ReadFile(filepath.Join(d.Dir, u.Rel))
		if err != nil {
			return "", "", err
		}
		updates[u.Rel] = data
	}
	gitWithContext := func(callCtx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(callCtx, "git", args...)
		cmd.Dir = repo
		return cmd.Output()
	}
	git := func(args ...string) ([]byte, error) { return gitWithContext(ctx, args...) }
	// with several jobs at once, two reports can be ready together: wait for the other commit (and
	// its push) rather than failing the job
	unlock, err := publishLock(ctx, repo)
	if err != nil {
		return "", "", err
	}
	defer unlock()
	content, err := os.ReadFile(filepath.Join(d.Dir, d.Rel))
	if err != nil {
		return "", "", err
	}
	marker := "Submission-Case: " + cid
	if previous, _ := git("log", "-1", "--format=%H", "--fixed-strings", "--grep="+marker, "--", d.Rel); len(bytes.TrimSpace(previous)) > 0 {
		sha := strings.TrimSpace(string(previous))
		committed, e := git("show", sha+":"+d.Rel)
		current, e2 := git("show", "HEAD:"+d.Rel)
		working, e3 := os.ReadFile(filepath.Join(repo, d.Rel))
		if e == nil && e2 == nil && e3 == nil && bytes.Equal(committed, content) && bytes.Equal(current, content) && bytes.Equal(working, content) {
			return sha, "", nil
		}
		return "", "", errors.New("the previously committed report has changed")
	}
	if d.PreviousRel == "" {
		if err := checkPublicationCoverage(repo, d, content); err != nil {
			return "", "", err
		}
	}
	paths := []string{d.Rel}
	if d.PreviousRel != "" && d.PreviousRel != d.Rel {
		paths = append(paths, d.PreviousRel)
	}
	for _, u := range d.Updates {
		paths = append(paths, u.Rel)
	}
	for _, path := range paths {
		// Reject symlinked report directories/files even when their target is inside the repo.
		for at := path; at != "."; at = filepath.Dir(at) {
			info, e := os.Lstat(filepath.Join(repo, at))
			if e == nil && info.Mode()&os.ModeSymlink != 0 {
				return "", "", errors.New("report path is a symlink")
			}
		}
		changes, e := git("status", "--porcelain", "--", path)
		if e != nil {
			return "", "", e
		}
		if len(changes) > 0 {
			return "", "", fmt.Errorf("report path has local edits: %s", path)
		}
	}
	// An update of an existing report may not replace a version changed since research began.
	bases := []string{}
	if d.PreviousRel != "" {
		bases = append(bases, d.PreviousRel)
	} else if _, e := os.Stat(filepath.Join(repo, d.Rel)); !os.IsNotExist(e) {
		return "", "", errors.New("new report would overwrite an existing file")
	}
	for _, u := range d.Updates {
		bases = append(bases, u.Rel)
	}
	for _, basePath := range bases {
		if d.Uploaded {
			current, e := git("rev-parse", "HEAD:"+basePath)
			if e != nil || d.BaseBlobs[basePath] == "" || strings.TrimSpace(string(current)) != d.BaseBlobs[basePath] {
				return "", "", &coverageChanged{country: "an existing report"}
			}
			continue
		}
		baseCmd := exec.CommandContext(ctx, "git", "show", "HEAD:"+basePath)
		baseCmd.Dir = d.Dir
		base, e := baseCmd.Output()
		current, e2 := git("show", "HEAD:"+basePath)
		if e != nil || e2 != nil || !bytes.Equal(base, current) {
			return "", "", &coverageChanged{country: "an existing report"}
		}
	}
	// Recheck under the publication lock: another job may have claimed a code
	// since this draft was prepared in its isolated snapshot.
	files := map[string]string{d.Rel: string(content)}
	for rel, data := range updates {
		files[rel] = string(data)
	}
	if err := reportcode.Check(repo, files, d.PreviousRel); err != nil {
		return "", "", err
	}
	original := map[string][]byte{}
	for _, path := range paths {
		data, e := os.ReadFile(filepath.Join(repo, path))
		if e == nil {
			original[path] = data
		} else if !os.IsNotExist(e) {
			return "", "", e
		}
	}
	rollback := func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = gitWithContext(rollbackCtx, append([]string{"reset", "--quiet", "HEAD", "--"}, paths...)...)
		for _, path := range paths {
			if data, ok := original[path]; ok {
				_ = os.WriteFile(filepath.Join(repo, path), data, 0644)
			} else {
				_ = os.Remove(filepath.Join(repo, path))
			}
		}
	}
	if err = os.MkdirAll(filepath.Dir(filepath.Join(repo, d.Rel)), 0755); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(filepath.Join(repo, d.Rel), content, 0644); err != nil {
		return "", "", err
	}
	for rel, data := range updates {
		if err = os.WriteFile(filepath.Join(repo, rel), data, 0644); err != nil {
			rollback()
			return "", "", err
		}
	}
	if d.PreviousRel != "" && d.PreviousRel != d.Rel {
		if err = os.Remove(filepath.Join(repo, d.PreviousRel)); err != nil {
			rollback()
			return "", "", err
		}
	}
	if _, err = git(append([]string{"add", "--"}, paths...)...); err != nil {
		rollback()
		return "", "", err
	}
	// the new photos' manifest entries and thumbnails go into the same commit
	committing, message, note := paths, commitTitle(d)+"\n\n"+marker, ""
	if media != nil {
		if files, err := media.apply(ctx, repo); err != nil {
			note = "Photo thumbnails were not added (" + err.Error() + ")."
		} else {
			if len(files) > 0 {
				committing = append(append([]string{}, paths...), files...)
				message += "\nSubmission-Media: " + cid
			}
			note = media.note()
		}
	}
	undo := func() {
		rollback()
		if media != nil {
			restoreMedia(repo)
		}
	}
	args := []string{"-c", "user.name=Submission service", "-c", "user.email=submissions@localhost", "-c", "commit.gpgsign=false", "commit", "--only", "-m", message, "--"}
	if _, err = git(append(args, committing...)...); err != nil {
		// A cancellation can race with a successful commit. Inspect Git using a fresh
		// context before restoring files, so an already committed report stays intact.
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		previous, e := gitWithContext(recoveryCtx, "log", "-1", "--format=%H", "--fixed-strings", "--grep="+marker, "--", d.Rel)
		if e == nil && len(bytes.TrimSpace(previous)) > 0 {
			sha := strings.TrimSpace(string(previous))
			committed, e := gitWithContext(recoveryCtx, "show", sha+":"+d.Rel)
			cancel()
			if e == nil && bytes.Equal(committed, content) {
				return sha, note, nil
			}
		} else {
			cancel()
		}
		undo()
		return "", "", errors.New("report commit failed")
	}
	sha, err := git("rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(sha)), note, nil
}

func commitTitle(d draftJob) string {
	title := "Add report: " + d.Title
	if d.SourceOnly {
		title = "Add source to report: " + d.Title
	}
	if n := len(d.Updates); n == 1 {
		title += " (and update " + d.Updates[0].Slug + ")"
	} else if n > 1 {
		title += fmt.Sprintf(" (and update %d reports)", n)
	}
	return title
}
