package safeurl

import "encoding/json"

// Metadata is provenance, never proof of an article's claims. Raw structured data
// remains in the private capture; only selected descriptive fields reach the UI.
type Metadata struct {
	Meta        map[string]string `json:"meta"`
	Keywords    string            `json:"keywords"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Canonical   string            `json:"canonical"`
	Language    string            `json:"language"`
	Authors     []string          `json:"authors"`
	PublishedAt string            `json:"published_at"`
	ModifiedAt  string            `json:"modified_at"`
	SiteName    string            `json:"site_name"`
	Robots      string            `json:"robots"`
	OpenGraph   map[string]string `json:"open_graph"`
	Twitter     map[string]string `json:"twitter"`
	JSONLD      []json.RawMessage `json:"json_ld"`
	Images      []SourceImage     `json:"images,omitempty"`
}

// SourceImage is a photo in the article, as the browser saw it: the private copy beside the
// screenshot in images/, where it is published, and its caption and credit.
type SourceImage struct {
	File    string `json:"file"`
	Src     string `json:"src"`
	Caption string `json:"caption,omitempty"`
	Credit  string `json:"credit,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
}

func (e Evidence) ReadableText() string {
	if e.Rendered {
		return e.Text
	}
	return Text(e.Content)
}
func (e Evidence) Supports(quote string) bool {
	if e.Rendered {
		return Contains(e.Text, quote)
	}
	return Contains(e.Content, quote)
}

type SourceMetadata struct {
	Description string   `json:"description,omitempty"`
	Canonical   string   `json:"canonical,omitempty"`
	Language    string   `json:"language,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	PublishedAt string   `json:"published_at,omitempty"`
	ModifiedAt  string   `json:"modified_at,omitempty"`
	SiteName    string   `json:"site_name,omitempty"`
}

func limitText(text string, max int) string {
	r := []rune(text)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return text
}
