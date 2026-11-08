package research

import "regexp"

// The research agent cannot see pictures; it judges them by caption, credit and article text. When
// its own caption says a picture is a file, stock or illustrative image, or that it is not known to
// show the event, the picture is a generic one (a gavel, handcuffs, a police sign, a city banner)
// and is left out of the new report. Only new media from a submission is checked: photos already
// in reports are never removed by this.
var stockCaption = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bnot (?:been )?(?:established|confirmed|verified|known|clear)\b.{0,40}?\b(?:show|shows|showing|depict|depicts|depicting)\b`),
	regexp.MustCompile(`(?i)\b(?:stock|file|archive|archival|library|illustrative|representative|symbolic|generic)\s+(?:photo|photograph|picture|image|illustration|footage)s?\b`),
	regexp.MustCompile(`(?i)\bused to illustrate\b|\bas an illustration\b|\bAI[- ]generated\b`),
	regexp.MustCompile(`(?i)\bdoes not (?:show|depict) (?:this|the) (?:event|seizure|arrest|case|raid|operation|defendant|suspect|accused|plants?|drugs?)\b`),
}

// StockCaption reports whether a caption admits that its picture is generic or not of this event.
func StockCaption(caption string) bool {
	for _, re := range stockCaption {
		if re.MatchString(caption) {
			return true
		}
	}
	return false
}

// DropStockMedia leaves out the photos and videos whose captions admit they are generic. It returns
// the media to keep and the captions of those left out.
func DropStockMedia(media []Media) ([]Media, []string) {
	kept, dropped := []Media{}, []string{}
	for _, m := range media {
		if StockCaption(m.Caption) {
			dropped = append(dropped, m.Caption)
			continue
		}
		kept = append(kept, m)
	}
	return kept, dropped
}
