package research

import (
	"bufio"
	"encoding/json"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/archivefile"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
)

// ReadToolEvidence reads only the harness's private execution log, never the
// model's final answer or search snippets. Completed webfetch bodies are reusable
// captures; every quotation still has to occur in that body. Truncated/failed
// captures fall back to the service's browser/HTTP and archive retrieval.
func ReadToolEvidence(dir string) []safeurl.Evidence {
	f, err := archivefile.Open(filepath.Join(dir, "research-events.jsonl"))
	if err != nil {
		return nil // Older/missing workspaces still have the initial checkpoint.
	}
	defer f.Close()
	scan := bufio.NewScanner(io.LimitReader(f, 16*1024*1024))
	scan.Buffer(make([]byte, 64*1024), 4*1024*1024)
	byURL := map[string]safeurl.Evidence{}
	bytes := 0
	for scan.Scan() {
		var event struct {
			Type string `json:"type"`
			Part struct {
				Tool  string `json:"tool"`
				State struct {
					Status   string                       `json:"status"`
					Error    string                       `json:"error"`
					Input    struct{ URL, Format string } `json:"input"`
					Output   string                       `json:"output"`
					Metadata map[string]any               `json:"metadata"`
				} `json:"state"`
			} `json:"part"`
		}
		if json.Unmarshal(scan.Bytes(), &event) != nil || event.Type != "tool_use" || event.Part.Tool != "webfetch" {
			continue
		}
		s := event.Part.State
		if s.Status != "completed" || s.Error != "" || len(s.Output) < 200 || len(s.Output) > 2*1024*1024 || truncated(s.Metadata) {
			continue
		}
		if s.Input.Format != "text" && s.Input.Format != "markdown" && s.Input.Format != "html" {
			continue
		}
		u, err := safeurl.Normalize(s.Input.URL)
		if err != nil {
			continue
		}
		e := safeurl.Evidence{URL: u, FinalURL: u, Content: s.Output}
		if s.Input.Format != "html" {
			e.Text = s.Output
		}
		if _, err := safeurl.Check(e); err != nil {
			continue
		}
		// Bound persisted checkpoint size as well as individual tool results.
		previous, exists := byURL[u]
		size := bytes - len(previous.Content) + len(e.Content)
		if (!exists && len(byURL) >= 32) || size > 4*1024*1024 {
			continue
		}
		byURL[u], bytes = e, size
	}
	values := make([]safeurl.Evidence, 0, len(byURL))
	urls := make([]string, 0, len(byURL))
	for u := range byURL {
		urls = append(urls, u)
	}
	sort.Strings(urls)
	for _, u := range urls {
		values = append(values, byURL[u])
	}
	return values
}

func truncated(metadata map[string]any) bool {
	for key, value := range metadata {
		if (strings.EqualFold(key, "truncated") || strings.EqualFold(key, "isTruncated")) && value == true {
			return true
		}
		if nested, ok := value.(map[string]any); ok && truncated(nested) {
			return true
		}
	}
	return false
}
