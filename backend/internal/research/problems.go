package research

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// The result check's messages are read by the research agent, which can only fix what they name:
// a bare "invalid fact" made it resubmit the same draft again and again. Each says which fact,
// source or field breaks which rule. Lengths are in characters, as the site's schema counts them.

func charCount(s string) int { return utf8.RuneCountInString(s) }

// factProblem is why a fact is invalid, or "". sources is how many sources the answer lists.
func factProblem(f Fact, sources int) string {
	switch {
	case f.Source < 1 || f.Source > sources:
		return fmt.Sprintf("its source is %d, but the answer lists %d source(s): number sources from 1 in the order of the sources list", f.Source, sources)
	case charCount(f.Text) < 10 || charCount(f.Text) > 2000:
		return fmt.Sprintf("its text must be 10–2000 characters (it has %d)", charCount(f.Text))
	case plain.MatchString(f.Text):
		return fmt.Sprintf("its text must be plain prose: remove %q (no < > [ ] ` * or control characters; the service adds citations itself)", plain.FindString(f.Text))
	case charCount(f.Quote) < 20 || charCount(f.Quote) > 4000:
		return fmt.Sprintf("its quote must be 20–4000 characters copied verbatim from the source (it has %d): quote the whole sentence", charCount(f.Quote))
	case f.Paragraph < 0 || f.Paragraph > 30:
		return fmt.Sprintf("its paragraph must be 0–30 (it is %d)", f.Paragraph)
	}
	return ""
}

// mergeProblem is why a fact's merge instructions are invalid, or "".
func mergeProblem(f Fact) string {
	switch {
	case f.ExistingText != "" && f.NewDetail:
		return "it has both existing_text and new_detail: use existing_text when the source supports a sentence the report already has, new_detail for a sentence the report lacks, not both"
	case charCount(f.ExistingText) > 2000:
		return fmt.Sprintf("its existing_text must be one sentence of the report, at most 2000 characters (it has %d)", charCount(f.ExistingText))
	case plain.MatchString(f.ExistingText):
		return fmt.Sprintf("its existing_text must be the report's sentence without citations or markup: remove %q", plain.FindString(f.ExistingText))
	}
	return ""
}

// excerpt shortens a fact text for a message.
func excerpt(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if charCount(s) > 60 {
		return string([]rune(s)[:57]) + "…"
	}
	return s
}

// textProblem checks a title or summary against the site's limits.
func textProblem(name, s string, max int) string {
	switch {
	case strings.TrimSpace(s) == "":
		return name + " is empty"
	case charCount(s) > max:
		return fmt.Sprintf("%s must be at most %d characters (it has %d): shorten it", name, max, charCount(s))
	case strings.Contains(s, "[@"):
		return name + " takes no citations ([@n]); cite in the facts"
	case plain.MatchString(s):
		return fmt.Sprintf("%s must be plain text: remove %q (no < > [ ] ` * or control characters)", name, plain.FindString(s))
	}
	return ""
}
