package research

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewDraftGetsServiceAdditionTimestamp(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "reports"), 0755)
	before := time.Now().UTC()
	d, err := PrepareDraft(dir, valid())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(d.Content, "updated_at:") {
		t.Fatal("new report incorrectly marked as updated")
	}
	const prefix = "added_at: "
	var value string
	for _, line := range strings.Split(d.Content, "\n") {
		if strings.HasPrefix(line, prefix) {
			value = strings.Trim(strings.TrimPrefix(line, prefix), `"`)
		}
	}
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || at.Before(before) || at.After(time.Now().UTC()) {
		t.Fatalf("invalid addition time %q: %v", value, err)
	}
	b, _ := json.Marshal(valid())
	b = append([]byte(`{"added_at":"2000-01-01T00:00:00Z",`), b[1:]...)
	if _, err := Strict(b); err == nil {
		t.Fatal("model-assigned addition time accepted")
	}
}

func TestAdditionTimeSurvivesExistingReportUpdate(t *testing.T) {
	dir := writeReport(t)
	rel := "reports/2026/10/khust-five-years.md"
	original := strings.Replace(existingReport, "---\n", "---\nadded_at: \"2026-10-28T10:00:00Z\"\nadded_at_source: git\n", 1)
	os.WriteFile(filepath.Join(dir, rel), []byte(original), 0644)
	u := ReportUpdate{Report: "khust-five-years", Title: "Corrected court headline", Facts: []Fact{{Text: "The Khust District Court sentenced a man to five years in prison.", Source: 1, Quote: "sentenced to five years in prison"}}}
	d, err := UpdateReport(dir, u, []Source{{URL: "https://second.example/story", Title: "Second account", Publisher: "Second"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Content, "added_at: \"2026-10-28T10:00:00Z\"") || !strings.Contains(d.Content, "added_at_source: git") {
		t.Fatal("addition metadata changed", d.Content)
	}
	if strings.Contains(d.Content, "updated_at:") {
		t.Fatal("update time must come from Git, not report front matter")
	}
}

func TestCheckAdditionTimestamp(t *testing.T) {
	r := valid()
	r.Facts = []Fact{{Text: strings.Repeat("Police raided the farm and arrested two men. ", 6), Source: 1, Quote: "q", Paragraph: 1}}
	for _, value := range []string{`"2026-11-03T12:45:37Z"`, `"2026-11-03T12:45:37.123Z"`} {
		md := strings.Replace(r.Markdown(), "---\n", "---\nadded_at: "+value+"\n", 1)
		if err := CheckReport(repo, "reports/2026/01/example-raid.md", md); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{`"2026-11-03"`, `"2026-11-03T12:45:37+01:00"`, `"2026-02-30T12:45:37Z"`, `123`} {
		md := strings.Replace(r.Markdown(), "---\n", "---\nadded_at: "+value+"\n", 1)
		if err := CheckReport(repo, "reports/2026/01/example-raid.md", md); err == nil {
			t.Fatal("invalid timestamp accepted", value)
		}
	}
}

func TestLegacyUpdateMaterializesOriginalAddition(t *testing.T) {
	dir := writeReport(t)
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2026-10-28T10:00:00Z", "GIT_COMMITTER_DATE=2026-10-28T10:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	}
	git("init")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.org")
	git("add", "reports")
	git("commit", "-m", "Original addition")
	u := ReportUpdate{Report: "khust-five-years", Title: "Corrected court headline", Facts: []Fact{{Text: "The Khust District Court sentenced a man to five years in prison.", Source: 1, Quote: "sentenced to five years in prison"}}}
	d, err := UpdateReport(dir, u, []Source{{URL: "https://second.example/story", Title: "Second account", Publisher: "Second"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Content, `added_at: "2026-10-28T10:00:00Z"`) || !strings.Contains(d.Content, "added_at_source: git") {
		t.Fatal("legacy addition lost", d.Content)
	}
}
