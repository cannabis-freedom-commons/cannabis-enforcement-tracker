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
// manifest change is committed together with the report, in one commit.
func TestReportCommitIncludesItsPhotoManifest(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	root, _ := filepath.Abs("../../..")
	// the backend CI job installs Go only; the script needs the site's packages (sharp, js-yaml, thumbhash)
	for _, pkg := range []string{"sharp", "js-yaml", "thumbhash"} {
		if _, err := os.Stat(filepath.Join(root, "node_modules", pkg, "package.json")); err != nil {
			t.Skip("site packages not installed (npm ci)")
		}
	}
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
	run("git", "init", "-q")
	run("git", "-c", "user.name=t", "-c", "user.email=t@t", "add", ".")
	run("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "init")

	// the validated draft, in the job's workspace
	workspace := t.TempDir()
	rel := "reports/2026/01/example.md"
	content := "---\ncode: 7KQ2MX\ntitle: T\nmedia:\n  - kind: photo\n    url: http://127.0.0.1/page\n    media_url: http://127.0.0.1/secret.jpg\n    publisher: P\n    caption: C\n  - kind: video\n    url: http://127.0.0.1/page\n    media_url: http://127.0.0.1/secret.mp4\n    publisher: P\n    caption: V\n---\n\nBody\n"
	os.MkdirAll(filepath.Join(workspace, filepath.Dir(rel)), 0755)
	os.WriteFile(filepath.Join(workspace, rel), []byte(content), 0644)
	d := draftJob{Dir: workspace, Rel: rel, Slug: "example", Title: "T"}

	media, err := prepareMedia(context.Background(), dir, workspace, []string{rel})
	if err != nil || media == nil {
		t.Fatal("no media prepared", err)
	}
	defer media.Close()
	sha, note, err := commitReportWithMedia(context.Background(), dir, "case1", d, media, false, "")
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
	if log := run("git", "log", "-1", "--format=%H%n%s%n%b"); !strings.HasPrefix(log, sha) || !strings.Contains(log, "Add report: T") || !strings.Contains(log, "Submission-Media: case1") {
		t.Fatal(log)
	}
	if files := run("git", "show", "--name-only", "--format=", "HEAD"); !strings.Contains(files, rel) || !strings.Contains(files, mediaManifest) {
		t.Fatal("one commit should hold the report and its manifest:", files)
	}
	if n := strings.TrimSpace(run("git", "rev-list", "--count", "HEAD")); n != "2" {
		t.Fatal("commits:", n)
	}
	if status := run("git", "status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Fatal("left changes:", status)
	}
	// the photos are known now: nothing new to prepare
	if again, err := prepareMedia(context.Background(), dir, workspace, []string{rel}); err != nil || again != nil {
		t.Fatal("prepared again", err)
	}

	// someone's uncommitted manifest edit: the next report is committed alone, the edit left as it was
	rel2 := "reports/2026/01/second.md"
	content2 := strings.Replace(strings.Replace(content, "7KQ2MX", "8MR3NY", 1), "secret.jpg", "other.jpg", 1)
	os.WriteFile(filepath.Join(workspace, rel2), []byte(content2), 0644)
	media2, err := prepareMedia(context.Background(), dir, workspace, []string{rel2})
	if err != nil || media2 == nil {
		t.Fatal("no media prepared", err)
	}
	defer media2.Close()
	edited := strings.Replace(string(b), "{", "{ ", 1)
	os.WriteFile(filepath.Join(dir, mediaManifest), []byte(edited), 0644)
	_, note, err = commitReportWithMedia(context.Background(), dir, "case2", draftJob{Dir: workspace, Rel: rel2, Slug: "second", Title: "Second"}, media2, false, "")
	if err != nil || !strings.Contains(note, "local changes") {
		t.Fatal(note, err)
	}
	if files := run("git", "show", "--name-only", "--format=", "HEAD"); strings.TrimSpace(files) != rel2 {
		t.Fatal("the report alone should be committed:", files)
	}
	if now, _ := os.ReadFile(filepath.Join(dir, mediaManifest)); string(now) != edited {
		t.Fatal("the local manifest edit was changed")
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
