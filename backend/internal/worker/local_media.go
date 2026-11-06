package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/frontmatter"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"gopkg.in/yaml.v3"
)

const mediaManifest = "src/data/media.json"

// publishLock serialises commits to the report repository (the reports, and their photo
// thumbnails) across jobs. It waits up to two minutes for another commit and its push.
func publishLock(ctx context.Context, repo string) (func(), error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--absolute-git-dir")
	cmd.Dir = repo
	gitDir, err := cmd.Output()
	if err != nil {
		return nil, errors.New("report repository is unavailable")
	}
	lock, err := os.OpenFile(filepath.Join(strings.TrimSpace(string(gitDir)), "submission-publish.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for waited := time.Duration(0); ; waited += 250 * time.Millisecond {
		if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); lock.Close() }, nil
		}
		if waited >= 2*time.Minute || ctx.Err() != nil {
			lock.Close()
			return nil, errors.New("another report is being committed")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// reportPhotos lists the photo addresses (media_url) the given committed reports link.
func reportPhotos(repo string, rels []string) []string {
	photos, _ := reportMedia(repo, rels)
	return photos
}

var (
	youtubeID = regexp.MustCompile(`^[\w-]{6,20}$`)
	videoFile = regexp.MustCompile(`(?i)\.(mp4|webm|ogv|mov|m4v)$`)
)

// videoPoster says how a video that plays in place gets its poster: YouTube's preview image, or a
// frame of the file (""); ok is false for a video shown only as a link (src/lib/media.js).
func videoPoster(raw string) (poster string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	if videoFile.MatchString(u.Path) {
		return "", true
	}
	host := strings.TrimPrefix(strings.TrimPrefix(u.Hostname(), "www."), "m.")
	id := ""
	switch {
	case host == "youtu.be":
		id = strings.TrimPrefix(u.Path, "/")
	case host == "youtube.com" || host == "youtube-nocookie.com":
		id = u.Query().Get("v")
		for _, prefix := range []string{"/embed/", "/shorts/", "/live/"} {
			if id == "" && strings.HasPrefix(u.Path, prefix) {
				id = strings.SplitN(strings.TrimPrefix(u.Path, prefix), "/", 2)[0]
			}
		}
	}
	if youtubeID.MatchString(id) {
		return "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg", true
	}
	return "", false
}

// reportMedia lists the photos and the videos that play in place the given reports link.
func reportMedia(repo string, rels []string) (photos, videos []string) {
	seen := map[string]bool{}
	for _, rel := range rels {
		b, err := os.ReadFile(filepath.Join(repo, rel))
		if err != nil {
			continue
		}
		parts := frontmatter.Split(string(b))
		if len(parts) != 3 {
			continue
		}
		var data struct {
			Media []struct {
				Kind     string `yaml:"kind"`
				MediaURL string `yaml:"media_url"`
			} `yaml:"media"`
		}
		if yaml.Unmarshal([]byte(parts[1]), &data) != nil {
			continue
		}
		for _, m := range data.Media {
			if !strings.HasPrefix(m.MediaURL, "http") || seen[m.MediaURL] {
				continue
			}
			if m.Kind == "photo" {
				seen[m.MediaURL] = true
				photos = append(photos, m.MediaURL)
			} else if _, ok := videoPoster(m.MediaURL); m.Kind == "video" && ok {
				seen[m.MediaURL] = true
				videos = append(videos, m.MediaURL)
			}
		}
	}
	return photos, videos
}

// frameOf takes one frame of a downloaded video start as JPEG with ffmpeg, which reads only the
// local file: a second in, or its first frame for a short clip.
func frameOf(ctx context.Context, video, out string) error {
	for _, at := range []string{"1", "0"} {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		cmd := exec.CommandContext(runCtx, "ffmpeg", "-nostdin", "-loglevel", "error", "-y", "-protocol_whitelist", "file", "-ss", at, "-i", video, "-frames:v", "1", "-f", "image2", "-vcodec", "mjpeg", out)
		err := cmd.Run()
		cancel()
		if info, e := os.Stat(out); err == nil && e == nil && info.Size() > 1000 {
			return nil
		}
	}
	return errors.New("no frame could be read")
}

// commitMedia gives the photos of a just-committed report their manifest entry, ThumbHash
// placeholder and 320px thumbnail (scripts/media-manifest.mjs), and commits those on their own:
// src/data/media.json and the new files in public/media/. The photos are fetched here, through the
// public-address-only client; the script never fetches. Best effort: a failure leaves the report
// committed and is returned as a note.
func commitMedia(ctx context.Context, repo, cid, title string, rels []string) (string, error) {
	photos, videos := reportMedia(repo, rels)
	if len(photos)+len(videos) == 0 {
		return "", nil
	}
	known := map[string]struct {
		Status string `json:"status"`
	}{}
	if b, err := os.ReadFile(filepath.Join(repo, mediaManifest)); err == nil {
		_ = json.Unmarshal(b, &known)
	}
	var todo []string
	isVideo := map[string]bool{}
	for _, u := range append(append([]string{}, photos...), videos...) {
		if _, ok := known[u]; !ok && len(todo) < 12 {
			todo = append(todo, u)
		}
	}
	for _, u := range videos {
		isVideo[u] = true
	}
	if len(todo) == 0 {
		return "", nil
	}
	tmp, err := os.MkdirTemp("", "tracker-media-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	type item struct {
		URL    string `json:"url"`
		Video  bool   `json:"video,omitempty"`
		Poster string `json:"poster,omitempty"`
		File   string `json:"file,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	var input []item
	for i, u := range todo {
		if isVideo[u] {
			input = append(input, videoItem(ctx, tmp, i, u))
			continue
		}
		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		b, err := safeurl.FetchImage(fetchCtx, u)
		cancel()
		if err != nil {
			input = append(input, item{URL: u, Error: err.Error()})
			continue
		}
		file := filepath.Join(tmp, fmt.Sprintf("%d", i))
		if err = os.WriteFile(file, b, 0600); err != nil {
			return "", err
		}
		input = append(input, item{URL: u, File: file})
	}
	inputFile := filepath.Join(tmp, "input.json")
	data, _ := json.Marshal(input)
	if err = os.WriteFile(inputFile, data, 0600); err != nil {
		return "", err
	}

	unlock, err := publishLock(ctx, repo)
	if err != nil {
		return "", err
	}
	defer unlock()
	git := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repo
		return cmd.Output()
	}
	paths := []string{mediaManifest, "public/media"}
	// someone else's uncommitted work on the manifest or thumbnails: leave it alone
	if dirty, err := git(append([]string{"status", "--porcelain", "--"}, paths...)...); err != nil || len(bytes.TrimSpace(dirty)) > 0 {
		return "", errors.New("the media manifest has local changes")
	}
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	script := exec.CommandContext(runCtx, "node", "scripts/media-manifest.mjs", "--input", inputFile)
	script.Dir = repo
	if out, err := script.CombinedOutput(); err != nil {
		restoreMedia(repo)
		return "", fmt.Errorf("thumbnails could not be made (%s)", lastLine(out, err))
	}
	if _, err = git(append([]string{"add", "--"}, paths...)...); err != nil {
		restoreMedia(repo)
		return "", err
	}
	staged, _ := git(append([]string{"diff", "--cached", "--name-only", "--"}, paths...)...)
	if len(bytes.TrimSpace(staged)) == 0 {
		return "", nil
	}
	args := []string{"-c", "user.name=Submission service", "-c", "user.email=submissions@localhost", "-c", "commit.gpgsign=false", "commit", "--only", "-m", "Add photo thumbnails: " + title + "\n\nSubmission-Media: " + cid, "--"}
	if _, err = git(append(args, strings.Fields(string(staged))...)...); err != nil {
		restoreMedia(repo)
		return "", errors.New("the thumbnails could not be committed")
	}
	ok := 0
	for _, it := range input {
		if it.File != "" {
			ok++
		}
	}
	return fmt.Sprintf("%d of %d new photos and videos got a thumbnail or poster.", ok, len(input)), nil
}

// videoItem gets a video's poster for the manifest script: YouTube's preview image, or a frame of
// the file's first 16 MB (fetched through the public-address-only client; ffmpeg reads only that).
func videoItem(ctx context.Context, tmp string, i int, u string) (it struct {
	URL    string `json:"url"`
	Video  bool   `json:"video,omitempty"`
	Poster string `json:"poster,omitempty"`
	File   string `json:"file,omitempty"`
	Error  string `json:"error,omitempty"`
}) {
	it.URL, it.Video = u, true
	poster, _ := videoPoster(u)
	fetchCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	file := filepath.Join(tmp, fmt.Sprintf("%d.jpg", i))
	if poster != "" {
		it.Poster = poster
		b, err := safeurl.FetchImage(fetchCtx, poster)
		if err == nil {
			err = os.WriteFile(file, b, 0600)
		}
		if err != nil {
			it.Error = err.Error()
			return it
		}
		it.File = file
		return it
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		it.Error = "ffmpeg is not installed"
		return it
	}
	b, err := safeurl.FetchVideoStart(fetchCtx, u, 16*1024*1024)
	video := filepath.Join(tmp, fmt.Sprintf("%d.video", i))
	if err == nil {
		err = os.WriteFile(video, b, 0600)
	}
	if err == nil {
		err = frameOf(ctx, video, file)
	}
	if err != nil {
		it.Error = err.Error()
		return it
	}
	it.File = file
	return it
}

// restoreMedia undoes an unfinished thumbnail run: the manifest as committed, no new files.
func restoreMedia(repo string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, args := range [][]string{{"reset", "--quiet", "HEAD", "--", mediaManifest, "public/media"}, {"checkout", "--", mediaManifest}, {"clean", "-fdq", "--", "public/media"}} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = repo
		_ = cmd.Run()
	}
}

func lastLine(out []byte, err error) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if s := strings.TrimSpace(lines[len(lines)-1]); s != "" {
		if len(s) > 160 {
			s = s[:160]
		}
		return s
	}
	return err.Error()
}
