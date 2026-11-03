// Package research treats model output as a draft proposal, never verified evidence.
package research

import (
	"unicode"

	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/reportcode"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Source struct {
	Archived string `json:"-"` // independently opened archive, assigned by the service

	URL       string `json:"url"`
	Title     string `json:"title"`
	Publisher string `json:"publisher"`
	Date      string `json:"date,omitempty"`     // when the source was published, YYYY-MM-DD
	Language  string `json:"language,omitempty"` // two letters, if not English
}
type Fact struct {
	confirmed    bool   // service-only quotation check; never accepted from JSON or persisted across retries
	Text         string `json:"text"`
	Source       int    `json:"source"`
	Quote        string `json:"quote"`
	Paragraph    int    `json:"paragraph,omitempty"`     // facts with the same number make one paragraph
	ExistingText string `json:"existing_text,omitempty"` // existing sentence corroborated by this source
	NewDetail    bool   `json:"new_detail,omitempty"`    // explicitly novel information in an existing report
}

// Media is a photo or video of the event that may not be copied to the site (a newspaper's, an
// agency's): a source-page link, the individual asset URL, its publisher and what it shows
// (docs/WRITING.md §7, src/content.config.ts).
type Media struct {
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	MediaURL  string `json:"media_url,omitempty"`
	Publisher string `json:"publisher"`
	Caption   string `json:"caption"`
}

type Money struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// Figures are the report's numbers, each only where a source states it (src/content.config.ts).
type Figures struct {
	Arrested        *int     `json:"arrested,omitempty"`
	Charged         *int     `json:"charged,omitempty"`
	KgSeized        *float64 `json:"kg_seized,omitempty"`
	PlantsDestroyed *int     `json:"plants_destroyed,omitempty"`
	Hectares        *float64 `json:"hectares,omitempty"`
	Prison          string   `json:"prison,omitempty"`
	Fine            *Money   `json:"fine,omitempty"`
	Value           *Money   `json:"value,omitempty"`
	Deaths          *int     `json:"deaths,omitempty"`
}
type Result struct {
	// Set by the service after execution, never accepted from model-generated JSON.
	Code           string    `json:"-"` // permanent CET id, allocated by the service
	AddedAt        string    `json:"-"` // first accepted draft timestamp, assigned by the service
	CreatedByModel string    `json:"-"`
	Submission     string    `json:"-"` // the submission that created the report, set by the service
	Outcome        string    `json:"outcome"`
	Country        string    `json:"country"`
	Date           string    `json:"incident_date"`
	Type           string    `json:"type"`
	Also           []string  `json:"also,omitempty"`
	Place          string    `json:"place,omitempty"`
	Coordinates    []float64 `json:"coordinates,omitempty"` // [lat, lon]
	Precision      string    `json:"precision,omitempty"`
	Figures        *Figures  `json:"figures,omitempty"`
	Tags           []string  `json:"tags,omitempty"`
	Media          []Media   `json:"media,omitempty"`
	Sources        []Source  `json:"sources"`
	Facts          []Fact    `json:"facts"`
	Slug           string    `json:"proposed_slug"`
	Title          string    `json:"title"`
	Summary        string    `json:"summary"`
	ExistingReport string    `json:"existing_report"`
	Explanation    string    `json:"explanation"`
	DecisionTitle  string    `json:"decision_title,omitempty"`
	Uncertainties  []string  `json:"uncertainties"`
	// Other existing reports this run changes (additional sources, open questions, corrections).
	Updates []ReportUpdate `json:"report_updates,omitempty"`
}

var slug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var plain = regexp.MustCompile("[<>\\[\\]`*\\x00-\\x08]")
var tag = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var currency = regexp.MustCompile(`^[A-Z]{3}$`)
var language = regexp.MustCompile(`^[a-z]{2}$`)
var submissionID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var types = "|eradication|seizure|raid|arrest|prosecution|sentence|execution|death|"
var precisions = "|exact|locality|district|region|country|"

func Strict(data []byte) (Result, error) {
	var r Result
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return r, err
	}
	if d.Decode(new(any)) != io.EOF {
		return r, errors.New("extra research output")
	}
	return r, nil
}

// CleanTags turns the model's tags into the form reports use (lower-case letters, digits and hyphens,
// at most 40 characters, at most 8): "Hong Kong" becomes hong-kong, accents are dropped, and a tag
// with nothing left (one in another script, such as 香港) is left out. Tags only help search, so a
// badly written one is never a reason to refuse a report.
func CleanTags(tags []string) []string {
	out, seen := []string{}, map[string]bool{}
	for _, t := range tags {
		folded, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), strings.ToLower(strings.TrimSpace(t)))
		if err != nil {
			continue
		}
		var b strings.Builder
		for _, r := range folded {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
				b.WriteRune(r)
			case r == '-' || r == ' ' || r == '_' || r == '/' || r == '.':
				b.WriteRune('-')
			}
		}
		clean := strings.Trim(regexp.MustCompile(`-+`).ReplaceAllString(b.String(), "-"), "-")
		if len(clean) > 40 {
			clean = strings.Trim(clean[:40], "-")
		}
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
		if len(out) == 8 {
			break
		}
	}
	return out
}

// CleanPresentation removes formatting from public decision labels, never from facts or
// quotations. Drafts can use a service-owned description of the proposed operation when
// the model omitted its explanation; a rejection still needs the model's actual rationale.
func (r *Result) CleanPresentation() {
	r.Explanation = strings.TrimSpace(plain.ReplaceAllString(r.Explanation, ""))
	r.DecisionTitle = strings.TrimSpace(plain.ReplaceAllString(r.DecisionTitle, ""))
	if r.Explanation == "" {
		switch r.Outcome {
		case "new_event":
			r.Explanation = "The AI proposed a new enforcement report for source and report validation."
		case "additional_source":
			r.Explanation = "The AI proposed adding sources to an existing report for source and report validation."
		case "existing_case":
			r.Explanation = "The AI proposed updating an existing case for source and report validation."
		}
	}
}

func (r Result) Validate(countries map[string]bool) error {
	switch r.Outcome {
	case "new_event", "existing_case", "additional_source", "duplicate", "not_relevant", "insufficient_evidence", "routed_to_cfi", "needs_review":
	default:
		return errors.New("invalid outcome")
	}
	if utf8.RuneCountInString(r.DecisionTitle) > 120 || plain.MatchString(r.DecisionTitle) {
		return errors.New("invalid decision title")
	}
	if n := utf8.RuneCountInString(r.Explanation); n < 10 || n > 1500 || plain.MatchString(r.Explanation) {
		return errors.New("invalid explanation")
	}
	if len(r.Sources) > 8 || len(r.Facts) > 30 || len(r.Uncertainties) > 30 {
		return errors.New("result exceeds bounds")
	}
	for _, question := range r.Uncertainties {
		if strings.TrimSpace(question) == "" || len(question) > 4000 || strings.ContainsAny(question, "\x00") {
			return errors.New("invalid open question")
		}
	}
	for _, s := range r.Sources {
		if _, err := safeurl.Normalize(s.URL); err != nil {
			return err
		}
		if s.Title == "" || s.Publisher == "" || len(s.Title) > 500 || len(s.Publisher) > 200 {
			return errors.New("invalid source metadata")
		}
		if s.Date != "" {
			if _, err := time.Parse("2006-01-02", s.Date); err != nil {
				return errors.New("invalid source date")
			}
		}
		if s.Language != "" && !language.MatchString(s.Language) {
			return errors.New("invalid source language")
		}
	}
	if (r.Outcome == "not_relevant" || r.Outcome == "routed_to_cfi") && len(r.Sources) == 0 {
		return errors.New("classification requires an opened source")
	}
	if !r.Drafts() {
		if len(r.Updates) > 0 {
			return errors.New("report updates need a drafted outcome")
		}
		if r.Outcome == "duplicate" && (!slug.MatchString(r.ExistingReport) || len(r.Sources) == 0) {
			return errors.New("duplicate requires an existing report and an opened source")
		}
		return nil
	}
	if len(r.Updates) > 5 {
		return errors.New("too many report updates")
	}
	updated := map[string]bool{r.Slug: r.Outcome != "additional_source", r.ExistingReport: r.Outcome == "additional_source"}
	cited := map[int]bool{}
	for _, f := range r.Facts {
		if err := validateMergeFact(f); err != nil {
			return err
		}
		cited[f.Source] = true
	}
	for _, u := range r.Updates {
		if err := u.validate(len(r.Sources), cited); err != nil {
			return err
		}
		if updated[u.Report] {
			return errors.New("a report is updated twice")
		}
		updated[u.Report] = true
	}
	if r.Outcome == "additional_source" {
		// another account of an event a report already records: its facts are the report's
		// sentences the new sources support
		if !slug.MatchString(r.ExistingReport) || len(r.Sources) == 0 {
			return errors.New("an additional source needs its existing report")
		}
		if err := r.validateMedia(); err != nil {
			return err
		}
		return r.Primary().validate(len(r.Sources), nil)
	}
	if strings.TrimSpace(r.CreatedByModel) == "" {
		return errors.New("creator model is required")
	}
	if r.Outcome == "existing_case" && (r.ExistingReport == "" || r.Slug != r.ExistingReport) {
		return errors.New("existing-case update must preserve its slug")
	}
	date, err := time.Parse("2006-01-02", r.Date)
	if err != nil || date.Year() < 2000 || date.After(time.Now().Add(24*time.Hour)) {
		return errors.New("invalid incident date")
	}
	if !countries[r.Country] || !slug.MatchString(r.Slug) || len(r.Slug) > 100 {
		return errors.New("invalid country or slug")
	}
	if !strings.Contains(types, "|"+r.Type+"|") {
		return errors.New("type must be routed or reviewed")
	}
	for _, t := range r.Also {
		if t == r.Type || !strings.Contains(types, "|"+t+"|") {
			return errors.New("invalid also type")
		}
	}
	if len(r.Coordinates) != 2 || r.Coordinates[0] < -90 || r.Coordinates[0] > 90 || r.Coordinates[1] < -180 || r.Coordinates[1] > 180 || !strings.Contains(precisions, "|"+r.Precision+"|") || r.Precision == "" {
		return errors.New("coordinates and precision are required")
	}
	if len(r.Place) > 200 || plain.MatchString(r.Place) {
		return errors.New("invalid place")
	}
	if len(r.Tags) > 8 {
		return errors.New("too many tags")
	}
	for _, t := range r.Tags {
		if !tag.MatchString(t) || len(t) > 40 {
			return fmt.Errorf("invalid tag %q", t)
		}
	}
	if err := r.Figures.validate(); err != nil {
		return err
	}
	if err := r.validateMedia(); err != nil {
		return err
	}
	if len(r.Title) == 0 || len(r.Title) > 120 || len(r.Summary) == 0 || len(r.Summary) > 400 || plain.MatchString(r.Title) || plain.MatchString(r.Summary) {
		return errors.New("invalid title or summary")
	}
	if len(r.Sources) == 0 || len(r.Facts) == 0 {
		return errors.New("missing evidence")
	}
	used := map[int]bool{}
	for _, f := range r.Facts {
		if f.Paragraph < 0 || f.Paragraph > 30 || f.Source < 1 || f.Source > len(r.Sources) || len(f.Text) < 10 || len(f.Text) > 2000 || len(f.Quote) < 20 || len(f.Quote) > 4000 || plain.MatchString(f.Text) {
			return errors.New("invalid fact or source reference")
		}
		used[f.Source] = true
	}
	if len(used) != len(r.Sources) {
		return errors.New("uncited source")
	}
	return nil
}

// Drafts says whether the answer changes the repository: a new report, an update of the existing
// case, or sources added to an existing report.
func (r Result) Drafts() bool {
	return r.Outcome == "new_event" || r.Outcome == "existing_case" || r.Outcome == "additional_source"
}

func (r Result) Verify(ctx context.Context, initial safeurl.Evidence) ([]safeurl.Evidence, error) {
	_, evidence, _, err := r.Confirm(ctx, initial, safeurl.Fetch)
	return evidence, err
}

// Validation is how an answer was checked, for the request page: each source, whether the service
// could open it again and how many of its quotations it found; the facts left out; the verdict.
type Validation struct {
	Sources   []SourceCheck `json:"sources"`
	Facts     int           `json:"facts"`     // facts in the answer
	Checked   int           `json:"checked"`   // facts whose source opened, so their quotation could be looked for
	Confirmed int           `json:"confirmed"` // of those, the ones whose quotation was found
	LeftOut   []string      `json:"left_out,omitempty"`
	Passed    bool          `json:"passed"`
	Reason    string        `json:"reason,omitempty"` // why it did not pass
}
type SourceCheck struct {
	Title     string `json:"title"`
	Publisher string `json:"publisher"`
	URL       string `json:"url"`
	Opened    bool   `json:"opened"`
	Error     string `json:"error,omitempty"`
	Facts     int    `json:"facts"`
	Confirmed int    `json:"confirmed"`
}

// Confirm reuses captured sources and opens uncaptured ones to check each quotation. Only a really bad
// answer is refused: one with no confirmed fact, or one where most quotations checked are missing
// (a sign of invented quotes). Otherwise the answer is kept, made sound and its gaps said:
//   - a source the service cannot open again (a site that blocks it with 403, a paywall) stays, with
//     its facts as the AI read them, and a note in the uncertainties;
//   - a fact whose quotation is missing from a source that did open is left out, with a note;
//   - a source no longer cited after that is left out, and the facts renumbered.
//
// It returns the answer to publish, the evidence that was opened and how it went.
func (r Result) Confirm(ctx context.Context, initial safeurl.Evidence, fetch func(context.Context, string) (safeurl.Evidence, error), cached ...safeurl.Evidence) (Result, []safeurl.Evidence, Validation, error) {
	// Recompute trust from evidence on every call, including checkpoint recovery.
	r.Facts = uncheckedFacts(r.Facts)
	r.Updates = append([]ReportUpdate(nil), r.Updates...)
	for i := range r.Updates {
		r.Updates[i].Facts = uncheckedFacts(r.Updates[i].Facts)
	}
	v := Validation{Facts: len(r.Facts), Sources: make([]SourceCheck, len(r.Sources))}
	fail := func(err error) (Result, []safeurl.Evidence, Validation, error) {
		v.Reason = err.Error()
		return r, nil, v, err
	}
	evidence := []safeurl.Evidence{}
	captures := map[string]safeurl.Evidence{}
	// The service's initial capture takes precedence over later tool reads.
	for _, e := range append(cached, initial) {
		if e.Content == "" || safeurl.AccessChallenge(e) != "" {
			continue
		}
		for _, raw := range []string{e.URL, e.FinalURL, e.ArchiveURL} {
			if u, err := safeurl.Normalize(raw); err == nil {
				captures[u] = e
			}
		}
	}
	opened := make([]*safeurl.Evidence, len(r.Sources))
	for i, s := range r.Sources {
		v.Sources[i] = SourceCheck{Title: s.Title, Publisher: s.Publisher, URL: s.URL}
		u, err := safeurl.Normalize(s.URL)
		if err != nil {
			return fail(err)
		}
		e, captured := captures[u]
		if !captured {
			if e, err = fetch(ctx, u); err != nil {
				if ctx.Err() != nil {
					return r, nil, v, ctx.Err()
				}
				v.Sources[i].Error = err.Error()
				r.Uncertainties = append(r.Uncertainties, fmt.Sprintf("%s (%s) could not be opened again to check its quotations (%s); what it says is as the AI read it.", s.Title, s.Publisher, strings.TrimSuffix(err.Error(), ".")))
				continue
			}
		}
		r.Sources[i].Archived = e.ArchiveURL
		v.Sources[i].Opened = true
		opened[i] = &e
		evidence = append(evidence, e)
	}
	kept := []Fact{}
	for _, f := range r.Facts {
		v.Sources[f.Source-1].Facts++
		e := opened[f.Source-1]
		if e == nil {
			kept = append(kept, f)
			continue
		}
		v.Checked++
		if e.Supports(f.Quote) {
			f.confirmed = true
			v.Confirmed++
			v.Sources[f.Source-1].Confirmed++
			kept = append(kept, f)
			continue
		}
		v.LeftOut = append(v.LeftOut, f.Text)
		r.Uncertainties = append(r.Uncertainties, fmt.Sprintf("Left out, as its quotation was not found in %s: %s", r.Sources[f.Source-1].Publisher, f.Text))
	}
	// the facts of updates to other reports: their quotations are checked the same way; an update
	// that loses all its facts is dropped
	updates := []ReportUpdate{}
	for _, u := range r.Updates {
		facts := []Fact{}
		for _, f := range u.Facts {
			e := opened[f.Source-1]
			if e != nil && !e.Supports(f.Quote) {
				v.LeftOut = append(v.LeftOut, f.Text)
				r.Uncertainties = append(r.Uncertainties, fmt.Sprintf("Not added to %s, as its quotation was not found in %s: %s", u.Report, r.Sources[f.Source-1].Publisher, f.Text))
				continue
			}
			f.confirmed = e != nil
			facts = append(facts, f)
		}
		if len(facts) > 0 {
			u.Facts = facts
			updates = append(updates, u)
		}
	}
	missing := v.Checked - v.Confirmed
	switch {
	case v.Confirmed == 0 && v.Checked > 0:
		return fail(errors.New("no supporting quotation was found in the opened sources"))
	case v.Confirmed == 0:
		return fail(errors.New("no source could be opened again to check the facts"))
	case missing*2 > v.Checked:
		return fail(fmt.Errorf("%d of %d quotations were not found in the opened sources", missing, v.Checked))
	}
	// the sources still cited, in their order, and the facts pointing at their new numbers
	cited := map[int]bool{}
	for _, f := range kept {
		cited[f.Source] = true
	}
	order, renumber := []Source{}, map[int]int{}
	for i, s := range r.Sources {
		if cited[i+1] {
			order = append(order, s)
			renumber[i+1] = len(order)
		}
	}
	for i := range kept {
		kept[i].Source = renumber[kept[i].Source]
	}
	r.Updates = nil
	for _, u := range updates {
		facts := []Fact{}
		for _, f := range u.Facts {
			if n := renumber[f.Source]; n != 0 {
				f.Source = n
				facts = append(facts, f)
			}
		}
		if len(facts) > 0 {
			u.Facts = facts
			r.Updates = append(r.Updates, u)
		}
	}
	r.Sources, r.Facts = order, kept
	v.Passed = true
	return r, evidence, v, nil
}

var (
	plantCount = regexp.MustCompile(`(?i)\b(\d[\d,.]*)\s+(?:[a-z-]+\s+){0,2}plants\b`)
	kgCount    = regexp.MustCompile(`(?i)\b(\d[\d,.]*)\s*(?:kg|kilograms?|kilos?)\b`)
	haCount    = regexp.MustCompile(`(?i)\b(\d[\d,.]*)\s*(?:ha|hectares?)\b`)
)

// FigureGaps says, as open questions, which counts the text gives that are missing from the
// figures: "52 cannabis plants" in the facts with no plants figure, a weight with no kg figure, an
// area with no hectares figure. It only points them out; a number may be about something else (a
// legal limit), so nothing is filled in.
func FigureGaps(r Result) []string {
	if r.Outcome != "new_event" && r.Outcome != "existing_case" {
		return nil
	}
	text := r.Summary
	for _, f := range r.Facts {
		text += " " + f.Text
	}
	f := r.Figures
	if f == nil {
		f = &Figures{}
	}
	var out []string
	check := func(missing bool, re *regexp.Regexp, what, field string) {
		if m := re.FindStringSubmatch(text); missing && m != nil {
			out = append(out, fmt.Sprintf("The text gives %s (%q) but the figures have no %s; check whether it belongs there.", what, strings.TrimSpace(m[0]), field))
		}
	}
	check(f.PlantsDestroyed == nil, plantCount, "a number of plants", "plants_destroyed")
	check(f.KgSeized == nil, kgCount, "a weight", "kg_seized")
	check(f.Hectares == nil, haCount, "an area", "hectares")
	return out
}

// a media link must be one of the report's own sources (a page about this event that was opened)
func (r Result) validateMedia() error {
	if len(r.Media) > 10 {
		return errors.New("too many media links")
	}
	cited := map[string]bool{}
	for _, s := range r.Sources {
		if u, err := safeurl.Normalize(s.URL); err == nil {
			cited[u] = true
		}
	}
	for _, m := range r.Media {
		u, err := safeurl.Normalize(m.URL)
		if err != nil || !cited[u] {
			return errors.New("a media link must be a page among the sources")
		}
		if mediaURL, err := safeurl.Normalize(m.MediaURL); err != nil || (m.Kind == "photo" && mediaURL == u) {
			return errors.New("collected media requires a valid media_url distinct from the source page")
		}
		if (m.Kind != "photo" && m.Kind != "video") || m.Publisher == "" || len(m.Publisher) > 200 || len(m.Caption) < 3 || len(m.Caption) > 300 || plain.MatchString(m.Caption) || plain.MatchString(m.Publisher) {
			return errors.New("invalid media link")
		}
	}
	return nil
}

func (f *Figures) validate() error {
	if f == nil {
		return nil
	}
	for _, n := range []*int{f.Arrested, f.Charged, f.PlantsDestroyed, f.Deaths} {
		if n != nil && *n < 0 {
			return errors.New("invalid figure")
		}
	}
	for _, n := range []*float64{f.KgSeized, f.Hectares} {
		if n != nil && *n < 0 {
			return errors.New("invalid figure")
		}
	}
	for _, m := range []*Money{f.Fine, f.Value} {
		if m != nil && (m.Amount <= 0 || !currency.MatchString(m.Currency)) {
			return errors.New("invalid money figure")
		}
	}
	if len(f.Prison) > 100 || plain.MatchString(f.Prison) {
		return errors.New("invalid prison figure")
	}
	return nil
}

// The report file, laid out like the hand-written ones in reports/: YAML front matter in the
// schema's order, then the text in paragraphs with a citation after each sentence.
func (r Result) Markdown() string {
	doc := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k string, v *yaml.Node) { doc.Content = append(doc.Content, str(k), v) }
	code := r.Code
	if code == "" {
		code = reportcode.Of(r.Slug)
	}
	add("code", str(code))
	if r.AddedAt != "" {
		add("added_at", str(r.AddedAt))
	}
	add("date", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!timestamp", Value: r.Date})
	add("country", str(r.Country))
	if r.Place != "" {
		add("place", str(r.Place))
	}
	if len(r.Coordinates) == 2 {
		add("coordinates", flow(num(r.Coordinates[0]), num(r.Coordinates[1])))
	}
	if r.Precision != "" {
		add("precision", str(r.Precision))
	}
	add("type", str(r.Type))
	if len(r.Also) > 0 {
		add("also", flow(strs(r.Also)...))
	}
	add("title", str(strings.TrimSpace(r.Title)))
	summary := str(strings.Join(strings.Fields(r.Summary), " "))
	summary.Style = yaml.FoldedStyle
	add("summary", summary)
	if f := r.Figures.node(); f != nil {
		add("figures", f)
	}
	if len(r.Media) > 0 {
		media := &yaml.Node{Kind: yaml.SequenceNode}
		for _, m := range r.Media {
			item := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{str("kind"), str(m.Kind), str("url"), str(m.URL)}}
			if m.MediaURL != "" {
				item.Content = append(item.Content, str("media_url"), str(m.MediaURL))
			}
			item.Content = append(item.Content, str("publisher"), str(m.Publisher), str("caption"), str(strings.Join(strings.Fields(m.Caption), " ")))
			media.Content = append(media.Content, item)
		}
		add("media", media)
	}
	status := "reported"
	if factsVerified(r.Facts) {
		status = "verified"
	}
	add("status", str(status))
	if r.CreatedByModel != "" {
		add("created_by_model", str(r.CreatedByModel))
	}
	if submissionID.MatchString(r.Submission) {
		add("submission", str(r.Submission))
	}
	if len(r.Tags) > 0 {
		add("tags", flow(strs(r.Tags)...))
	}
	sources := &yaml.Node{Kind: yaml.SequenceNode}
	for _, s := range r.Sources {
		m := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{str("title"), str(s.Title), str("url"), str(s.URL), str("publisher"), str(s.Publisher)}}
		if s.Archived != "" {
			m.Content = append(m.Content, str("archived"), str(s.Archived))
		}
		if s.Date != "" {
			m.Content = append(m.Content, str("date"), &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!timestamp", Value: s.Date})
		}
		if s.Language != "" && s.Language != "en" {
			m.Content = append(m.Content, str("language"), str(s.Language))
		}
		sources.Content = append(sources.Content, m)
	}
	add("sources", sources)
	if len(r.Uncertainties) > 0 {
		add("open_questions", &yaml.Node{Kind: yaml.SequenceNode, Content: strs(r.Uncertainties)})
	}
	var head bytes.Buffer
	enc := yaml.NewEncoder(&head)
	enc.SetIndent(2)
	_ = enc.Encode(doc)
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(head.String())
	b.WriteString("---\n\n")
	b.WriteString(Body(r.Facts, func(n int) int { return n }))
	return b.String()
}

// Body is the report text: facts grouped into their paragraphs, each sentence followed by its
// citation, wrapped at 80 columns. source maps a fact's source number to the file's.
func Body(facts []Fact, source func(int) int) string {
	var paragraphs [][]string
	last := -1
	for i, f := range facts {
		// a fact without a paragraph number starts a new paragraph, as older answers had it
		if i == 0 || f.Paragraph == 0 || f.Paragraph != last {
			paragraphs = append(paragraphs, nil)
		}
		last = f.Paragraph
		paragraphs[len(paragraphs)-1] = append(paragraphs[len(paragraphs)-1], fmt.Sprintf("%s [@%d]", strings.TrimSpace(f.Text), source(f.Source)))
	}
	var b strings.Builder
	for _, p := range paragraphs {
		b.WriteString(wrap(strings.Join(p, " "), 80))
		b.WriteString("\n\n")
	}
	return b.String()
}

func wrap(text string, width int) string {
	var lines []string
	line := ""
	for _, w := range strings.Fields(text) {
		if line != "" && len([]rune(line))+1+len([]rune(w)) > width {
			lines = append(lines, line)
			line = w
		} else if line == "" {
			line = w
		} else {
			line += " " + w
		}
	}
	return strings.Join(append(lines, line), "\n")
}

func str(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }
func num(v float64) *yaml.Node {
	value := strconv.FormatFloat(v, 'f', -1, 64)
	if strings.Contains(value, ".") {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: value}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: value}
}
func integer(v int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}
}
func strs(v []string) []*yaml.Node {
	out := []*yaml.Node{}
	for _, s := range v {
		out = append(out, str(s))
	}
	return out
}
func flow(items ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle, Content: items}
}
func (f *Figures) node() *yaml.Node {
	if f == nil {
		return nil
	}
	m := &yaml.Node{Kind: yaml.MappingNode}
	ints := []struct {
		k string
		v *int
	}{{"arrested", f.Arrested}, {"charged", f.Charged}, {"plants_destroyed", f.PlantsDestroyed}, {"deaths", f.Deaths}}
	floats := []struct {
		k string
		v *float64
	}{{"kg_seized", f.KgSeized}, {"hectares", f.Hectares}}
	for _, x := range ints {
		if x.v != nil {
			m.Content = append(m.Content, str(x.k), integer(*x.v))
		}
	}
	for _, x := range floats {
		if x.v != nil {
			m.Content = append(m.Content, str(x.k), num(*x.v))
		}
	}
	if f.Prison != "" {
		m.Content = append(m.Content, str("prison"), str(f.Prison))
	}
	for _, x := range []struct {
		k string
		v *Money
	}{{"fine", f.Fine}, {"value", f.Value}} {
		if x.v != nil {
			m.Content = append(m.Content, str(x.k), &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{str("amount"), num(x.v.Amount), str("currency"), str(x.v.Currency)}})
		}
	}
	if len(m.Content) == 0 {
		return nil
	}
	return m
}

type Runner struct {
	Config config.Config
	Store  *store.Store
	// A publication race needs a new event-identity decision, not a new source crawl.
	PreviousResult   *Result
	PreviousEvidence []safeurl.Evidence
	ReviewProblem    string
}

// Snapshot clones history and overlays trusted current site inputs for local validation.
func (r Runner) Snapshot(ctx context.Context, cid string) (string, error) {
	root, err := filepath.Abs(r.Config.Workspaces)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, cid+"-")
	if err != nil {
		return "", err
	}
	prepared := false
	defer func() {
		if !prepared {
			_ = os.RemoveAll(dir)
		}
	}()
	repo, err := filepath.Abs(r.Config.RepoPath)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--no-hardlinks", "--local", "--", repo, dir)
	if err = cmd.Run(); err != nil {
		return "", errors.New("isolated checkout failed")
	}
	if err = snapshotSite(ctx, repo, dir); err != nil {
		return "", err
	}
	// Only the explicit evidence input is exposed to OpenCode, never this checkout.
	prepared = true
	return dir, nil
}

type capped struct {
	bytes.Buffer
	Max int
}

func (b *capped) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.Max {
		return 0, errors.New("output limit")
	}
	return b.Buffer.Write(p)
}
func (r Runner) Run(ctx context.Context, cid, dir, run string, e safeurl.Evidence, coverage []byte) (result Result, runErr error) {
	var events *eventWriter
	if err := r.Store.StartRun(ctx, run, r.Config.ResearchModel, r.Config.ResearchRunner); err != nil {
		return Result{}, err
	}
	if err := r.Store.AddAIEvent(ctx, cid, run, "user", "request", "Investigate the submitted source, confirm the event and follow the tracker editorial rules."); err != nil {
		return Result{}, err
	}
	defer func() {
		endCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		status := "completed"
		if runErr != nil {
			status = "failed"
			if errors.Is(ctx.Err(), context.Canceled) {
				status = "interrupted"
			}
			// say why, in words: the data is public, only credentials are redacted
			why := runErr.Error()
			if events != nil {
				why = events.safe(why)
			}
			_ = r.Store.AddAIEvent(endCtx, cid, run, "system", "error", "The OpenCode run could not complete: "+why+". Processing will retry automatically within its limits.")
		}
		_ = r.Store.EndRun(endCtx, run, status, result.Outcome)
	}()

	_, err := r.Store.DB.ExecContext(ctx, "UPDATE research_runs SET workspace=? WHERE id=?", dir, run)
	if err != nil {
		return Result{}, err
	}
	// Bound model input independently of the larger private evidence archive.
	if len(e.Content) > 100000 {
		e.Content = e.Content[:100000]
	}
	input := filepath.Join(dir, ".research-input")
	if err = os.Mkdir(input, 0755); err != nil {
		return Result{}, err
	}
	// Mount an explicit allowlist, not the repository's .git, environment, or production DB.
	var reportRows []json.RawMessage
	if err = json.Unmarshal(coverage, &reportRows); err != nil {
		return Result{}, err
	}
	var index strings.Builder
	for _, row := range reportRows {
		index.Write(row)
		index.WriteByte('\n')
	}
	// The page goes in as files with short lines: the read tool cuts lines at 2,000 characters, so a
	// page inside one JSON line reached the model cut off. source.txt is the readable text (what
	// quotations are checked against, besides the markup), source.html the markup for details.
	text := e.ReadableText()
	request := map[string]any{"case_id": cid, "url": e.URL, "final_url": e.FinalURL, "existing_reports_index": "reports-index.jsonl", "initial_source_unavailable": e.Content == "", "example_reports": "examples/"}
	if e.ArchiveURL != "" {
		request["archive_url"] = e.ArchiveURL
		request["archive_timestamp"] = e.ArchivedAt
	}
	files := map[string][]byte{"reports-index.jsonl": []byte(index.String())}
	if r.PreviousResult != nil {
		request["coverage_review"] = true
		if r.ReviewProblem != "" {
			request["validation_feedback"] = r.ReviewProblem
		}
		request["previous_result"] = "previous-result.json"
		request["saved_sources"] = "saved-sources.json"
		files["previous-result.json"], _ = json.MarshalIndent(r.PreviousResult, "", "  ")
		var sources []map[string]string
		for i, source := range r.PreviousEvidence {
			text := source.ReadableText()
			if len(text) > 100000 {
				text = text[:100000]
			}
			name := fmt.Sprintf("saved-sources/%d.txt", i+1)
			files[name] = []byte(safeurl.Wrap(text, 1000) + "\n")
			sources = append(sources, map[string]string{"url": source.URL, "final_url": source.FinalURL, "text_file": name})
		}
		files["saved-sources.json"], _ = json.MarshalIndent(sources, "", "  ")
	}
	// The model needs the actual prose to merge another outlet's account without
	// paraphrasing the same facts into a second copy of the report.
	reportFiles, err := existingReportFiles(dir)
	if err != nil {
		return Result{}, err
	}
	for name, content := range reportFiles {
		files[name] = content
	}
	request["existing_reports_dir"] = "existing-reports/"
	// the article's photos the capture kept, with caption and credit: candidates for media links
	if e.Metadata != nil && len(e.Metadata.Images) > 0 {
		var images []map[string]string
		for _, img := range e.Metadata.Images {
			images = append(images, map[string]string{"src": img.Src, "caption": img.Caption, "credit": img.Credit})
		}
		request["source_images"] = images
	}
	if e.Content != "" {
		request["source_text"] = "source.txt"
		request["source_html"] = "source.html"
		request["source_text_lines"] = strings.Count(safeurl.Wrap(text, 1000), "\n") + 1
		files["source.txt"] = []byte(safeurl.Wrap(text, 1000) + "\n")
		files["source.html"] = []byte(safeurl.Wrap(strings.ReplaceAll(e.Content, "><", ">\n<"), 1000) + "\n")
	}
	if e.Metadata != nil {
		request["source_metadata"] = "source-metadata.json"
		metadata, _ := json.MarshalIndent(e.Metadata, "", "  ")
		files["source-metadata.json"] = metadata
	}
	if (e.Screenshot == "page.png" || e.Screenshot == "page.avif") && e.ArtifactDir != "" {
		name := e.Screenshot
		screenshot, err := os.ReadFile(filepath.Join(e.ArtifactDir, name))
		if os.IsNotExist(err) && name == "page.png" {
			name = "page.avif"
			screenshot, err = os.ReadFile(filepath.Join(e.ArtifactDir, name))
		}
		if err == nil {
			target := "source" + filepath.Ext(name)
			request["source_screenshot"] = target
			files[target] = screenshot
		}
	}
	request["browser_rendered"] = e.Rendered
	files["request.json"] = []byte(store.JSON(request))
	examples, err := exampleReports(r.Config.RepoPath, 3)
	if err != nil {
		return Result{}, err
	}
	for name, b := range examples {
		files["examples/"+name] = b
	}
	for _, path := range []string{"docs/CFI.md", "docs/WRITING.md", ".claude/skills/add-report/SKILL.md", ".claude/skills/find-reports/SKILL.md", ".opencode/agents/enforcement-researcher.md", "src/data/countries.json"} {
		b, err := os.ReadFile(filepath.Join(r.Config.RepoPath, path))
		if err != nil {
			return Result{}, err
		}
		if path == "src/data/countries.json" {
			var countries []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if err = json.Unmarshal(b, &countries); err != nil {
				return Result{}, err
			}
			b = []byte(store.JSON(countries))
		}
		if path == ".opencode/agents/enforcement-researcher.md" && r.Config.ResearchMaxSteps > 0 {
			b = regexp.MustCompile(`(?m)^steps: \d+$`).ReplaceAll(b, []byte(fmt.Sprintf("steps: %d", r.Config.ResearchMaxSteps)))
		}
		files[path] = b
	}
	files["opencode.json"] = r.modelConfig()
	for path, b := range files {
		target := filepath.Join(input, path)
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return Result{}, err
		}
		if err = os.WriteFile(target, b, 0644); err != nil {
			return Result{}, err
		}
	}
	cmd, cleanup, err := r.command(ctx, input, run)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	out := &capped{Max: 4 * 1024 * 1024}
	key := r.Config.ResearchKey
	if localKey := selectedProviderKey(cmd); localKey != "" {
		key = localKey
	}
	events = &eventWriter{ctx: ctx, store: r.Store, cid: cid, run: run, secret: key}
	raw, err := os.OpenFile(filepath.Join(dir, "research-events.jsonl"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return Result{}, err
	}
	defer raw.Close()
	diagnostics, err := os.OpenFile(filepath.Join(dir, "research-stderr.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return Result{}, err
	}
	defer diagnostics.Close()
	stderr := &capped{Max: 256 * 1024}
	cmd.Stdout = io.MultiWriter(out, raw, events)
	cmd.Stderr = io.MultiWriter(stderr, diagnostics)
	err = cmd.Run()
	if err != nil {
		return Result{}, runnerFailure(ctx, err, r.Config.ResearchTimeout, events.steps, stderr.String())
	}
	data, err := FinalText(out.Bytes())
	if err != nil {
		if err.Error() == "missing final result" {
			return Result{}, fmt.Errorf("OpenCode stopped after %d model steps without a final answer (the limit is %d steps)", events.steps, r.Config.ResearchMaxSteps)
		}
		return Result{}, err
	}
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return Result{}, fmt.Errorf("OpenCode ended after %d model steps with notes instead of the JSON answer the tracker needs (the limit is %d steps; it probably ran out of them)", events.steps, r.Config.ResearchMaxSteps)
	}
	result, err = Strict(data)
	if err != nil {
		return Result{}, fmt.Errorf("the final answer was rejected by the result check (%v)", err)
	}
	if result.Explanation != "" {
		if err = r.Store.AddAIEvent(ctx, cid, run, "assistant", "result", events.safe(result.Explanation)); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

// Why the OpenCode process ended badly, for the public chat: a timeout, a stop, or its exit code
// with the last line it printed.
func runnerFailure(ctx context.Context, err error, timeout time.Duration, steps int, stderr string) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("it ran out of time after %s and %d model steps", timeout.Round(time.Second), steps)
	case ctx.Err() != nil:
		return fmt.Errorf("it was stopped after %d model steps", steps)
	}
	msg := fmt.Sprintf("reading OpenCode's output failed after %d model steps (%v)", steps, err)
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		msg = fmt.Sprintf("OpenCode exited with code %d after %d model steps", exit.ExitCode(), steps)
	}
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		if r := []rune(last); len(r) > 300 {
			last = string(r[len(r)-300:])
		}
		msg += ": " + last
	}
	return errors.New(msg)
}

// OpenCode's --format json emits events; it is not the result JSON itself.
func FinalText(stream []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(stream))
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	var final strings.Builder
	var artifact []byte
	for scanner.Scan() {
		var e struct {
			Type string `json:"type"`
			Part struct {
				Text string `json:"text"`
			} `json:"part"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, errors.New("invalid execution stream")
		}
		if e.Type == "error" {
			var withError struct {
				Error struct {
					Name string `json:"name"`
					Data struct {
						Message string `json:"message"`
					} `json:"data"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal(scanner.Bytes(), &withError)
			why := strings.TrimSpace(withError.Error.Data.Message + " " + withError.Error.Message)
			if why == "" {
				why = withError.Error.Name
			}
			if why == "" {
				return nil, errors.New("OpenCode reported an error during the run")
			}
			return nil, fmt.Errorf("OpenCode reported an error: %s", why)
		}
		if e.Type == "text" {
			final.WriteString(e.Part.Text)
			candidate := []byte(strings.TrimSpace(e.Part.Text))
			if json.Valid(candidate) && bytes.HasPrefix(candidate, []byte("{")) {
				artifact = append([]byte(nil), candidate...)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(artifact) > 0 {
		return artifact, nil
	}
	text := strings.TrimSpace(final.String())
	if text == "" {
		return nil, errors.New("missing final result")
	}
	// the model sometimes writes a few words before its JSON answer: take the last complete object
	if object := lastJSONObject(text); object != nil {
		return object, nil
	}
	return []byte(text), nil // plain text (the startup probe's "OK"); a research run needs JSON
}

// The last complete JSON object in a text (the answer after any prose), or nil.
func lastJSONObject(text string) []byte {
	var found []byte
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(text[i:]))
		var raw json.RawMessage
		if dec.Decode(&raw) == nil && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
			found = append([]byte(nil), raw...)
			i += int(dec.InputOffset()) - 1
		}
	}
	return found
}

// exampleReports picks the newest finished reports (with a place, coordinates and figures) as
// models of the file the researcher's answer becomes.
func exampleReports(repo string, n int) (map[string][]byte, error) {
	var paths []string
	err := filepath.WalkDir(filepath.Join(repo, "reports"), func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	out := map[string][]byte{}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		s := string(b)
		if strings.Contains(s, "\nplace:") && strings.Contains(s, "\ncoordinates:") && strings.Contains(s, "\nfigures:") && strings.Contains(s, "\nstatus: verified") && !strings.Contains(s, "\ncover:") {
			out[filepath.Base(path)] = b
		}
		if len(out) == n {
			break
		}
	}
	return out, nil
}

func SaveEvidence(dir string, values []safeurl.Evidence) error {
	for i, e := range values {
		b := []byte(store.JSON(e))
		sum := sha256.Sum256(b)
		name := fmt.Sprintf("evidence-%d-%s.json", i+1, hex.EncodeToString(sum[:8]))
		if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			return err
		}
	}
	return nil
}

// Draft is a report file the run writes; Updates are the other existing reports it changes.
type Draft struct {
	Path, PreviousPath, Content, Slug, Title string
	Updates                                  []Draft
}

// PrepareDraft changes only the proposed report and the reports the answer updates, in the
// disposable checkout. An update that would change nothing is left out; for an additional source
// that is ErrUnchanged.
func PrepareDraft(dir string, r Result) (Draft, error) {
	var d Draft
	var err error
	if r.Outcome == "additional_source" {
		d, err = UpdateReport(dir, r.Primary(), r.Sources, r.Media)
	} else {
		d, err = prepareReport(dir, r)
		d.Slug = r.Slug
		if d.Title == "" {
			d.Title = r.Title
		}
	}
	if err != nil {
		return d, err
	}
	for _, u := range r.Updates {
		update, err := UpdateReport(dir, u, r.Sources, nil)
		if errors.Is(err, ErrUnchanged) {
			continue
		}
		if err != nil {
			return d, err
		}
		d.Updates = append(d.Updates, update)
	}
	return d, nil
}

// prepareReport writes a new report, or updates the existing case. Existing-case updates retain
// source numbering, original prose, assets and the report's stable slug.
func prepareReport(dir string, r Result) (Draft, error) {
	if strings.TrimSpace(r.CreatedByModel) == "" {
		return Draft{}, errors.New("creator model is required")
	}
	rel := filepath.ToSlash(filepath.Join("reports", r.Date[:4], r.Date[5:7], r.Slug+".md"))
	d := Draft{Path: rel, Content: r.Markdown()}
	var existing string
	err := filepath.WalkDir(filepath.Join(dir, "reports"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == r.Slug+".md" {
			if existing != "" {
				return errors.New("ambiguous existing slug")
			}
			existing = path
		}
		return nil
	})
	if err != nil {
		return d, err
	}
	if r.Outcome == "existing_case" {
		return updateCaseReport(dir, r)
	} else if existing != "" {
		return d, errors.New("slug already exists; case matching required")
	}
	used, err := reportcode.Index(dir)
	if err != nil {
		return d, err
	}
	if r.Code == "" || used[r.Code] != "" {
		r.Code = reportcode.Available(r.Slug, used)
	}
	if r.AddedAt == "" {
		r.AddedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	d.Content = r.Markdown()
	return d, nil
}

// buildLeftovers are what checking a draft with the site build leaves in a workspace: about 300 MB
// of dependencies, npm cache and built pages, none of it needed once the check is over.
var buildLeftovers = []string{"node_modules", "dist", ".npm", ".astro", ".cache"}

// PruneBuild removes a workspace's build leftovers, keeping the draft and the repository copy the
// commit step reads.
func PruneBuild(dir string) {
	for _, name := range buildLeftovers {
		_ = os.RemoveAll(filepath.Join(dir, name))
	}
}

// KeptFile says what a finished request's workspace keeps: the AI run's logs (the raw log view and
// the token count read them), the build check's log and the evidence files.
func KeptFile(name string) bool {
	name = strings.TrimSuffix(name, ".zst")
	switch name {
	case "research-events.jsonl", "research-stderr.log", "validation.log":
		return true
	}
	return strings.HasPrefix(name, "evidence-") && strings.HasSuffix(name, ".json")
}

// PruneWorkspace empties a finished request's workspace down to its logs and evidence: the
// repository copy, the research input and any session left by an interrupted run go.
func PruneWorkspace(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !(e.Type().IsRegular() && KeptFile(e.Name())) {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateBuild writes the draft and the reports it updates into the disposable checkout and checks
// each file as the site will read it (CheckReport): the front matter against the content schema's
// rules and the text against the report tests. The site itself is not built: a report is valid on
// its own, and the rest of the site (its tests, its dev server) has no say in it.
func ValidateBuild(ctx context.Context, dir string, r Result) (Draft, error) {
	defer PruneBuild(dir) // leftovers of an earlier build check, if any
	d, err := PrepareDraft(dir, r)
	if err != nil {
		return d, err
	}
	target := filepath.Join(dir, d.Path)
	if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return d, err
	}
	if err = os.WriteFile(target, []byte(d.Content), 0644); err != nil {
		return d, err
	}
	if d.PreviousPath != "" && d.PreviousPath != d.Path {
		if err = os.Remove(filepath.Join(dir, d.PreviousPath)); err != nil {
			return d, err
		}
	}
	for _, u := range d.Updates {
		if err = os.WriteFile(filepath.Join(dir, u.Path), []byte(u.Content), 0644); err != nil {
			return d, err
		}
	}
	var problems []string
	files := map[string]string{}
	for _, f := range append([]Draft{d}, d.Updates...) {
		files[f.Path] = f.Content
		if err := CheckReport(dir, f.Path, f.Content); err != nil {
			problems = append(problems, f.Path+": "+err.Error())
		}
	}
	if err := reportcode.Check(dir, files, d.PreviousPath); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		detail := strings.Join(problems, "\n")
		_ = os.WriteFile(filepath.Join(dir, "validation.log"), []byte(detail+"\n"), 0600)
		return d, &BuildError{Step: "report check", Detail: detail}
	}
	return d, nil
}

// BuildError is a draft whose report files failed the report check; Detail says which fields and
// citations, per file.
type BuildError struct{ Step, Detail string }

func (e *BuildError) Error() string {
	if e.Detail == "" {
		return "draft did not pass the " + e.Step
	}
	return "draft did not pass the " + e.Step + ":\n" + e.Detail
}
