package research

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrackerToolsInPublicChat(t *testing.T) {
	w := &eventWriter{dir: "/Users/someone/Projects/x/backend/data/jobs/abc-1"}
	in := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

	got := w.toolInput("tracker_find_reports", in(map[string]any{"country": "ru", "date": "2026-11-06", "title": "10 kg cache", "place": "Innokentyevka", "kg_seized": 10}))
	if got != "ru · 2026-11-06 · 10 kg cache · Innokentyevka" {
		t.Fatalf("find input: %q", got)
	}
	if got := w.toolInput("tracker_get_report", in(map[string]string{"slug": "primorye-stepnoe-22kg"})); got != "primorye-stepnoe-22kg" {
		t.Fatalf("get input: %q", got)
	}
	if got := w.toolInput("tracker_check_result", in(map[string]string{"result": "{\"title\":\"x\"}"})); got != "" {
		t.Fatalf("the draft must not be shown: %q", got)
	}
	// grep shows its pattern, and no host path
	got = w.toolInput("grep", in(map[string]string{"pattern": "Lixeira", "path": "/Users/someone/Projects/x/backend/data/jobs/abc-1/.research-input/existing-reports"}))
	if got != "Lixeira (existing-reports)" {
		t.Fatalf("grep input: %q", got)
	}
	got = w.toolInput("read", in(map[string]string{"path": "/Users/other/elsewhere/data/jobs/zzz-9/.research-input/source.html"}))
	if got != "source.html" {
		t.Fatalf("read input: %q", got)
	}
	if got := w.safe("see /Users/someone/secret/file.txt now"); strings.Contains(got, "/Users") {
		t.Fatalf("path leaked: %q", got)
	}

	// a place ending in -sk is not a credential; a key still is
	if got := w.safe("tomarinsky-krasnogorsk-shed-wild-cannabis-trial"); strings.Contains(got, "redacted") {
		t.Fatalf("slug redacted: %q", got)
	}
	if got := w.safe("key sk-abcdefghijklmnopqrstuv"); !strings.Contains(got, "[redacted]") {
		t.Fatalf("key shown: %q", got)
	}

	// a source address keeps its path
	if got := w.safe("Opening https://news.example/data/2026/11/raid.html"); got != "Opening https://news.example/data/2026/11/raid.html" {
		t.Fatalf("url changed: %q", got)
	}

	out := func(s string) json.RawMessage { b, _ := json.Marshal(s); return b }
	found := toolOutcome("tracker_find_reports", out("The result check will require a comparison with each of these (event_comparisons): read them with get_report.\n- a-slug | 2026 | ru | t\n\nOther possible matches, best first:\n- b | x\n- c | x\n- d | x\n"))
	if found != "must compare a-slug; 3 other possible matches, best b, c" {
		t.Fatalf("find outcome: %q", found)
	}
	if got := toolOutcome("tracker_find_reports", out("The result check requires no comparison for these details.\n\nNo other report in ru shares these words.\n")); got != "no matching report" {
		t.Fatalf("empty find: %q", got)
	}
	if got := toolOutcome("tracker_get_report", out("---\ncode: ABC123\ntitle: \"Over 136 kg seized\"\n---\nBody")); got != "“Over 136 kg seized”" {
		t.Fatalf("get outcome: %q", got)
	}
	if got := toolOutcome("tracker_check_result", out("The service would reject this result. Fix every problem below, then check again:\n- one\n- two")); got != "2 problems to fix" {
		t.Fatalf("check outcome: %q", got)
	}
	if got := toolOutcome("tracker_check_result", out("No problems found. Return this same JSON result as your final answer.")); got != "no problems found" {
		t.Fatalf("check ok: %q", got)
	}
}
