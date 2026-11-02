package research

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/klauspost/compress/zstd"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
)

func TestToolCapturesAreReusedButQuotationsAreStillChecked(t *testing.T) {
	r := valid()
	r.Sources = append(r.Sources, Source{URL: "https://news.example/second", Title: "Second source", Publisher: "News"})
	r.Facts = []Fact{{Text: "Police raided the farm.", Source: 1, Quote: "Police raided the farm in the province."}, {Text: "Two men were arrested.", Source: 2, Quote: "Two men were arrested at the farm."}}
	initial := safeurl.Evidence{URL: r.Sources[0].URL + "?utm_source=test", Content: r.Facts[0].Quote}
	dir := t.TempDir()
	entry := map[string]any{"type": "tool_use", "part": map[string]any{"tool": "webfetch", "state": map[string]any{"status": "completed", "input": map[string]string{"url": r.Sources[1].URL, "format": "text"}, "output": strings.Repeat(r.Facts[1].Quote+" ", 15)}}}
	b, _ := json.Marshal(entry)
	os.WriteFile(filepath.Join(dir, "research-events.jsonl"), append(b, '\n'), 0600)
	captured := ReadToolEvidence(dir)
	if len(captured) != 1 {
		t.Fatalf("lost completed capture: %d", len(captured))
	}
	encoder, _ := zstd.NewWriter(nil)
	os.WriteFile(filepath.Join(dir, "research-events.jsonl.zst"), encoder.EncodeAll(append(b, '\n'), nil), 0600)
	encoder.Close()
	os.Remove(filepath.Join(dir, "research-events.jsonl"))
	captured = ReadToolEvidence(dir)
	if len(captured) != 1 {
		t.Fatal("compressed tool evidence lost")
	}
	calls := 0
	fetch := func(context.Context, string) (safeurl.Evidence, error) {
		calls++
		return safeurl.Evidence{}, errors.New("now blocked")
	}
	_, evidence, check, err := r.Confirm(context.Background(), initial, fetch, captured...)
	if err != nil || calls != 0 || len(evidence) != 2 || check.Confirmed != 2 {
		t.Fatal(err, calls, check)
	}
	r.Facts[1].Quote = "This quotation was invented and never appeared in the article."
	_, _, check, err = r.Confirm(context.Background(), initial, fetch, captured...)
	if err != nil || check.Confirmed != 1 || len(check.LeftOut) != 1 {
		t.Fatal("cached capture bypassed quote validation", err, check)
	}
	_, _, _, _ = r.Confirm(context.Background(), initial, fetch)
	if calls != 1 {
		t.Fatal("uncaptured source did not use normal retrieval", calls)
	}
}

func TestToolEvidenceExcludesUntrustedIncompleteAndBlockedOutput(t *testing.T) {
	body := strings.Repeat("Police arrested two people and seized cannabis at the farm. ", 15)
	for _, tc := range []struct {
		name, kind, tool, status, url, format, body string
		metadata                                    map[string]any
	}{
		{"search snippet", "tool_use", "websearch", "completed", "https://news.example/a", "text", body, nil},
		{"model answer", "text", "webfetch", "completed", "https://news.example/a", "text", body, nil},
		{"failed fetch", "tool_use", "webfetch", "error", "https://news.example/a", "text", body, nil},
		{"private target", "tool_use", "webfetch", "completed", "http://127.0.0.1/a", "text", body, nil},
		{"truncated", "tool_use", "webfetch", "completed", "https://news.example/a", "text", body, map[string]any{"metadata": map[string]any{"truncated": true}}},
		{"unsupported format", "tool_use", "webfetch", "completed", "https://news.example/a", "image", body, nil},
		{"challenge", "tool_use", "webfetch", "completed", "https://news.example/a", "html", "<title>Just a moment...</title>" + body, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			b, _ := json.Marshal(map[string]any{"type": tc.kind, "part": map[string]any{"tool": tc.tool, "state": map[string]any{"status": tc.status, "input": map[string]string{"url": tc.url, "format": tc.format}, "output": tc.body, "metadata": tc.metadata}}})
			os.WriteFile(filepath.Join(dir, "research-events.jsonl"), b, 0600)
			if got := ReadToolEvidence(dir); len(got) != 0 {
				t.Fatal("unsafe evidence reused", got)
			}
		})
	}
}
