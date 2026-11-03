// Package frontmatter splits Markdown headers at delimiter lines, never inline hyphens.
package frontmatter

import "regexp"

var delimiter = regexp.MustCompile(`(?m)^---\r?$`)

// Split returns the prefix, YAML header and body, retaining their original bytes.
// The opening delimiter must start the document; nil means a delimiter is missing.
// Newlines surrounding the YAML and following the closing delimiter are preserved.
func Split(content string) []string {
	bounds := delimiter.FindAllStringIndex(content, 2)
	if len(bounds) != 2 || bounds[0][0] != 0 {
		return nil
	}
	return []string{"", content[3:bounds[1][0]], content[bounds[1][0]+3:]}
}
