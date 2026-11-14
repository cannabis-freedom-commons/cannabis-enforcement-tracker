package safeurl

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
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
	Metadata       *SourceMetadata `json:"metadata,omitempty"`
	Title          string          `json:"title"`
	Host           string          `json:"host"`
	Chars          int             `json:"chars"`
	Mention        string          `json:"mention"`
}

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
	c := SourceCheck{CaptureID: e.CaptureID, Host: host, Rendered: e.Rendered, Screenshot: e.Screenshot == "page.png" || e.Screenshot == "page.avif" || e.Screenshot == "page.webp", BrowserWarning: e.BrowserWarning, Cleanup: e.Cleanup, AccessBlocked: AccessChallenge(e), ArchiveURL: e.ArchiveURL, ArchivedAt: e.ArchivedAt}
	if e.Metadata != nil {
		m := e.Metadata
		c.Metadata = &SourceMetadata{Description: limitText(m.Description, 500), Canonical: limitText(m.Canonical, 4096), Language: limitText(m.Language, 40), Authors: m.Authors, PublishedAt: limitText(m.PublishedAt, 100), ModifiedAt: limitText(m.ModifiedAt, 100), SiteName: limitText(m.SiteName, 200)}
		c.Title = limitText(m.Title, 150)
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
	if q == "" {
		return false
	}
	text := strings.Join(strings.Fields(Text(content)), " ")
	if strings.Contains(text, q) {
		return true
	}
	// the same words typed differently: a quotation copied with straight quotes from a page with
	// curly ones (or the reverse), another dash, "..." for "…", or without a soft hyphen still counts
	fq := FoldQuote(q)
	return fq != "" && strings.Contains(FoldQuote(text), fq)
}

var quoteMarks = strings.NewReplacer(
	"\u2018", "'", "\u2019", "'", "\u201a", "'", "\u201b", "'", "\u2032", "'", "\u00b4", "'", "`", "'",
	"\u201c", "\"", "\u201d", "\"", "\u201e", "\"", "\u201f", "\"", "\u2033", "\"", "\u00ab", "\"", "\u00bb", "\"", "\u2039", "'", "\u203a", "'",
	"\u2010", "-", "\u2011", "-", "\u2012", "-", "\u2013", "-", "\u2014", "-", "\u2015", "-", "\u2212", "-",
	"\u2026", "...", "\u00ad", "", "\u200b", "", "\u200c", "", "\u200d", "", "\u200e", "", "\u200f", "", "\u2060", "", "\ufeff", "",
)

// FoldQuote is a quotation as its words and punctuation, whatever characters typed them: Unicode
// compatibility forms (NFKC), typographic quotation marks, apostrophes and dashes as plain ones, an
// ellipsis as three dots, invisible characters left out and whitespace collapsed.
func FoldQuote(s string) string {
	return strings.Join(strings.Fields(quoteMarks.Replace(norm.NFKC.String(s))), " ")
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

// Passage is a line or sentence of a source's text and the share (0 to 1) of a quotation's words
// it contains.
type Passage struct {
	Text  string  `json:"text"`
	Score float64 `json:"score"`
}

// Passages ranks the passages of the content (each line of its text, or the sentences of a long
// line) by how many of the quotation's words they share, best first, at most n of them, so a check
// or a tool can show the model what the source actually says instead of letting it search.
func Passages(content, quote string, n int) []Passage {
	words := func(s string) []string {
		return strings.FieldsFunc(strings.ToLower(FoldQuote(s)), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.Is(unicode.Mn, r) && !unicode.Is(unicode.Mc, r)
		})
	}
	want := words(quote)
	if len(want) == 0 || n < 1 {
		return nil
	}
	var out []Passage
	for _, line := range strings.Split(Text(content), "\n") {
		parts := []string{line}
		if utf8.RuneCountInString(line) > 400 {
			parts = sentenceEnd.Split(line, -1)
		}
		for _, part := range parts {
			part = strings.Join(strings.Fields(part), " ")
			have := map[string]int{}
			for _, w := range words(part) {
				have[w]++
			}
			found := 0
			for _, w := range want {
				if have[w] > 0 {
					have[w]--
					found++
				}
			}
			if found > 0 {
				out = append(out, Passage{Text: part, Score: float64(found) / float64(len(want))})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Closest is the content's best passage for a quotation (see Passages) and its score.
func Closest(content, quote string) (string, float64) {
	if p := Passages(content, quote, 1); len(p) > 0 {
		return p[0].Text, p[0].Score
	}
	return "", 0
}

var sentenceEnd = regexp.MustCompile(`(?:[.!?।]["'”’»]?)\s+`)

// Closest is the source's closest passage to a quotation it does not contain (see Closest).
func (e Evidence) Closest(quote string) (string, float64) {
	if e.Rendered {
		return Closest(e.Text, quote)
	}
	return Closest(e.Content, quote)
}
