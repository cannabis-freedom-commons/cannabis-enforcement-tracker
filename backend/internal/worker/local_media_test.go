package worker

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A report's photo at a private address is never fetched: it is recorded as unavailable, and the
// manifest change is committed on its own, after the report.
func TestCommitMediaFetchesOnlyPublicPhotosAndCommitsTheManifest(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	root, _ := filepath.Abs("../../..")
	dir := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal(args, err, string(out))
		}
		return string(out)
	}
	for _, name := range []string{"scripts/media-manifest.mjs", "src/lib/media.js", "package.json"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755)
		os.WriteFile(filepath.Join(dir, name), b, 0644)
	}
	if err := os.Symlink(filepath.Join(root, "node_modules"), filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "src/data"), 0755)
	os.MkdirAll(filepath.Join(dir, "public/media"), 0755)
	os.WriteFile(filepath.Join(dir, mediaManifest), []byte("{}\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\narchive/\n"), 0644)
	rel := "reports/2026/01/example.md"
	os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0755)
	os.WriteFile(filepath.Join(dir, rel), []byte("---\ntitle: T\nmedia:\n  - kind: photo\n    url: http://127.0.0.1/page\n    media_url: http://127.0.0.1/secret.jpg\n    publisher: P\n    caption: C\n  - kind: video\n    url: http://127.0.0.1/page\n    media_url: http://127.0.0.1/secret.mp4\n    publisher: P\n    caption: V\n---\n\nBody\n"), 0644)
	run("git", "init", "-q")
	run("git", "-c", "user.name=t", "-c", "user.email=t@t", "add", ".")
	run("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "init")

	note, err := commitMedia(context.Background(), dir, "case1", "T", []string{rel})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note, "0 of 2") {
		t.Fatal(note)
	}
	var m map[string]map[string]any
	b, _ := os.ReadFile(filepath.Join(dir, mediaManifest))
	if json.Unmarshal(b, &m) != nil || m["http://127.0.0.1/secret.jpg"]["status"] != "unavailable" || m["http://127.0.0.1/secret.mp4"]["status"] != "unavailable" || m["http://127.0.0.1/secret.mp4"]["kind"] != "video" {
		t.Fatal(string(b))
	}
	if log := run("git", "log", "-1", "--format=%s%n%b"); !strings.Contains(log, "Add photo thumbnails: T") || !strings.Contains(log, "Submission-Media: case1") {
		t.Fatal(log)
	}
	if status := run("git", "status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Fatal("left changes:", status)
	}
	// nothing new the second time
	if note, err = commitMedia(context.Background(), dir, "case1", "T", []string{rel}); err != nil || note != "" {
		t.Fatal(note, err)
	}
}

func TestVideoPosterSources(t *testing.T) {
	for raw, want := range map[string]string{
		"https://www.youtube.com/watch?v=_7bn_HBDbT8":  "https://i.ytimg.com/vi/_7bn_HBDbT8/hqdefault.jpg",
		"https://youtu.be/abcDEF12345":                 "https://i.ytimg.com/vi/abcDEF12345/hqdefault.jpg",
		"https://www.youtube.com/shorts/u2ZKouYEp3E":   "https://i.ytimg.com/vi/u2ZKouYEp3E/hqdefault.jpg",
		"https://cdn.example/clip.MP4":                 "",
		"https://rutube.ru/video/123/":                 "-",
		"https://www.youtube.com/watch?v=../../secret": "-",
	} {
		poster, ok := videoPoster(raw)
		if want == "-" {
			if ok {
				t.Fatal("accepted", raw)
			}
			continue
		}
		if !ok || poster != want {
			t.Fatal(raw, poster, ok)
		}
	}
}

func TestFrameOfReadsALocalVideo(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	video, frame := filepath.Join(dir, "clip.mp4"), filepath.Join(dir, "frame.jpg")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=10", "-movflags", "+faststart", video).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	if err := frameOf(context.Background(), video, frame); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(frame); len(b) < 1000 || b[0] != 0xFF || b[1] != 0xD8 {
		t.Fatal("not a JPEG frame")
	}
}
