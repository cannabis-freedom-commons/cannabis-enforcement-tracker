package safeurl

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SourceCheck is what the service found on the submitted page before any AI research: its title,
// where it lives, how much readable text it has, and the word for cannabis it uses, if any.
type SourceCheck struct {
	AccessBlocked  string          `json:"access_blocked,omitempty"`
	ArchiveURL     string          `json:"archive_url,omitempty"`
	ArchivedAt     string          `json:"archived_at,omitempty"`
	CaptureID      string          `json:"capture_id,omitempty"`
	Rendered       bool            `json:"rendered"`
	Screenshot     bool            `json:"screenshot"`
	BrowserWarning string          `json:"browser_warning,omitempty"`
	Cleanup        *Cleanup        `json:"cleanup,omitempty"`
	Images         []SourceImage   `json:"images,omitempty"` // the article's photos, kept privately
	Metadata       *SourceMetadata `json:"metadata,omitempty"`
	Title          string          `json:"title"`
	Host           string          `json:"host"`
	Chars          int             `json:"chars"`
	Mention        string          `json:"mention"`
}

var imageFile = regexp.MustCompile(`^images/[1-8]\.png$`)

var (
	dropBlocks = regexp.MustCompile(`(?is)<(script|style|noscript|svg|template)\b.*?</(script|style|noscript|svg|template)\s*>`)
	tags       = regexp.MustCompile(`(?s)<[^>]*>`)
	spaces     = regexp.MustCompile(`\s+`)
	titleTag   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

// Check reads the fetched page as text and says what it is. It fails when there is too little
// readable text to research (a page that needs JavaScript or a login).
func Check(e Evidence) (SourceCheck, error) {
	host := e.FinalURL
	if e.ArchiveURL != "" {
		host = e.URL
	}
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		host = strings.TrimPrefix(u.Hostname(), "www.")
	}
	c := SourceCheck{CaptureID: e.CaptureID, Host: host, Rendered: e.Rendered, Screenshot: e.Screenshot == "page.png" || e.Screenshot == "page.avif", BrowserWarning: e.BrowserWarning, Cleanup: e.Cleanup, AccessBlocked: AccessChallenge(e), ArchiveURL: e.ArchiveURL, ArchivedAt: e.ArchivedAt}
	if e.Metadata != nil {
		m := e.Metadata
		c.Metadata = &SourceMetadata{Description: limitText(m.Description, 500), Canonical: limitText(m.Canonical, 4096), Language: limitText(m.Language, 40), Authors: m.Authors, PublishedAt: limitText(m.PublishedAt, 100), ModifiedAt: limitText(m.ModifiedAt, 100), SiteName: limitText(m.SiteName, 200)}
		c.Title = limitText(m.Title, 150)
		for i, img := range m.Images {
			if i == 8 || !imageFile.MatchString(img.File) {
				break
			}
			c.Images = append(c.Images, SourceImage{File: img.File, Src: limitText(img.Src, 2000), Caption: limitText(img.Caption, 400), Credit: limitText(img.Credit, 120), Width: img.Width, Height: img.Height})
		}
	}
	if m := titleTag.FindStringSubmatch(e.Content); m != nil {
		c.Title = strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(tags.ReplaceAllString(m[1], " ")), " "))
		if utf8.RuneCountInString(c.Title) > 150 {
			c.Title = string([]rune(c.Title)[:150]) + "…"
		}
	}
	text := strings.Join(strings.Fields(e.ReadableText()), " ")
	c.Chars = utf8.RuneCountInString(text)
	if c.AccessBlocked != "" {
		return c, fmt.Errorf("%w: %s blocked access to the article", ErrAccessChallenge, c.AccessBlocked)
	}
	if c.Chars < 200 {
		if e.Rendered {
			return c, errors.New("the browser-rendered page has almost no readable article text (it may require a login, show an access challenge, or contain only media)")
		}
		return c, errors.New("the page has almost no readable text (it may need JavaScript or a login)")
	}
	c.Mention = cannabisMention(c.Title + " " + text)
	return c, nil
}

var (
	blockTags  = regexp.MustCompile(`(?i)<(?:/?(?:p|div|h[1-6]|li|tr|article|section|blockquote|figcaption|header|footer|table|ul|ol)\b[^>]*|br\s*/?)>`)
	lineSpaces = regexp.MustCompile(`[ \t\r\f\v\x{a0}]+`)
	blankLines = regexp.MustCompile(`\n\s*\n+`)
)

// Text is a page's readable text: scripts, styles and markup removed, entities decoded, one
// paragraph per line. The researcher reads it as a file, and quotations are checked against it.
func Text(content string) string {
	text := dropBlocks.ReplaceAllString(content, " ")
	text = blockTags.ReplaceAllString(text, "\n")
	text = html.UnescapeString(tags.ReplaceAllString(text, " "))
	var lines []string
	for _, line := range strings.Split(blankLines.ReplaceAllString(text, "\n"), "\n") {
		if line = strings.TrimSpace(lineSpaces.ReplaceAllString(line, " ")); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// Wrap breaks lines longer than max runes at a space (or anywhere, for scripts without spaces),
// so a tool that cuts long lines still shows the whole text.
func Wrap(text string, max int) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		r := []rune(line)
		for len(r) > max {
			cut := max
			for i := max; i > max/2; i-- {
				if r[i] == ' ' {
					cut = i
					break
				}
			}
			out = append(out, strings.TrimSpace(string(r[:cut])))
			r = []rune(strings.TrimSpace(string(r[cut:])))
		}
		out = append(out, string(r))
	}
	return strings.Join(out, "\n")
}

// Contains says whether a quotation appears on the page, in its markup or in its readable text,
// ignoring differences in spacing and line breaks.
func Contains(content, quote string) bool {
	if strings.Contains(content, quote) {
		return true
	}
	q := strings.Join(strings.Fields(html.UnescapeString(quote)), " ")
	return q != "" && strings.Contains(strings.Join(strings.Fields(Text(content)), " "), q)
}

// Describe says in one line what the check found, for the request's history.
func (c SourceCheck) Describe() string {
	title := ""
	if c.Title != "" {
		title = fmt.Sprintf("“%s”, ", c.Title)
	}
	if c.Mention == "" {
		return fmt.Sprintf("The source loaded: %s%s, %s characters of text. No known cannabis term detected; AI will check relevance.", title, c.Host, thousands(c.Chars))
	}
	return fmt.Sprintf("The source loaded: %s%s, %s characters of text, mentioning cannabis (%s).", title, c.Host, thousands(c.Chars), c.Mention)
}

func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
