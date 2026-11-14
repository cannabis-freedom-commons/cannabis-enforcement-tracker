package research

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
)

const findQuoteDescription = "Find the exact wording of a quotation in the captured sources (the submitted page and every " +
	"page you opened with webfetch), the text the service checks quotations against. Pass the words you remember or a " +
	"paraphrase, in the source's language; it returns the closest sentences exactly as the source has them, to copy into " +
	"quote fields. Use it instead of grep or reading source.txt to find a sentence."

var findQuoteSchema = map[string]any{"type": "object", "properties": map[string]any{
	"text":   map[string]any{"type": "string", "description": "The quotation as you remember it, or its key words."},
	"source": map[string]any{"type": "string", "description": "Optional: the URL of the source to search; all captured sources when left out."},
}, "required": []string{"text"}}

// findQuote answers the find_quote tool: the captured sources' passages closest to the text, best
// first, each with its source URL.
func (l *reportLookup) findQuote(raw json.RawMessage) string {
	var args struct {
		Text   string `json:"text"`
		Source string `json:"source"`
	}
	if json.Unmarshal(raw, &args) != nil || strings.TrimSpace(args.Text) == "" {
		return "Pass the quotation or its key words as text."
	}
	type hit struct {
		url string
		safeurl.Passage
	}
	var hits []hit
	for _, e := range capturedEvidence(l.dir) {
		if args.Source != "" && !sameSource(args.Source, e) {
			continue
		}
		content := e.Content
		if e.Rendered {
			content = e.Text
		}
		for _, p := range safeurl.Passages(content, args.Text, 3) {
			hits = append(hits, hit{e.URL, p})
		}
	}
	if len(hits) == 0 {
		if args.Source != "" {
			return "No captured source with that URL has these words. Leave source out to search every captured source, or open the page with webfetch first."
		}
		return "No captured source has these words. A quotation must come from the submitted page or a page you opened with webfetch."
	}
	// best first, at most 5, each only once
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Score > hits[j-1].Score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	var b strings.Builder
	b.WriteString("The closest passages, exactly as the sources have them (copy the words you need without changing them):\n")
	seen := map[string]bool{}
	n := 0
	for _, h := range hits {
		if n == 5 || h.Score < 0.2 {
			break
		}
		if seen[h.Text] {
			continue
		}
		seen[h.Text] = true
		n++
		text := h.Text
		if r := []rune(text); len(r) > 600 {
			text = string(r[:600]) + "…"
		}
		fmt.Fprintf(&b, "\n%d. %s (%.0f%% of your words)\n   source: %s\n", n, text, h.Score*100, h.url)
	}
	if n == 0 {
		return "No captured passage shares enough of these words. A quotation must come from the submitted page or a page you opened with webfetch."
	}
	return b.String()
}

// capturedEvidence: the run's captured pages, the submitted one and those saved before this run
// (evidence-*.json) and the webfetch bodies of this run.
func capturedEvidence(dir string) []safeurl.Evidence {
	var out []safeurl.Evidence
	files, _ := filepath.Glob(filepath.Join(dir, "evidence-*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var e safeurl.Evidence
		if json.Unmarshal(b, &e) == nil && (e.Content != "" || e.Text != "") {
			out = append(out, e)
		}
	}
	return append(out, ReadToolEvidence(dir)...)
}

func sameSource(url string, e safeurl.Evidence) bool {
	norm := func(u string) string {
		u = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://"), "/")
		return strings.TrimPrefix(u, "www.")
	}
	return norm(url) == norm(e.URL) || (e.FinalURL != "" && norm(url) == norm(e.FinalURL))
}
