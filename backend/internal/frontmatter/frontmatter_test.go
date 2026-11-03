package frontmatter

import (
	"strings"
	"testing"
)

func TestSplitOnlyAtDelimiterLines(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		header := newline + "title: \"news----worldwide\"" + newline + "summary: |" + newline + "  ---" + newline
		body := newline + "Article.\n\n---\nMore text.\n"
		content := "---" + header + "---" + body
		parts := Split(content)
		if len(parts) != 3 || parts[1] != header || parts[2] != body || strings.Join(parts, "---") != content {
			t.Fatalf("split changed header or body: %#v", parts)
		}
	}
	for _, content := range []string{"title: plain", "---\ntitle: missing close", "prefix\n---\ntitle: x\n---\n", "---inline\ntitle: x\n---\n"} {
		if parts := Split(content); parts != nil {
			t.Fatalf("accepted missing delimiter: %#v", parts)
		}
	}
}
