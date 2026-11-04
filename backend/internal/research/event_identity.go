package research

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ReportIndex keeps search metadata separate from the authoritative Markdown.
// Body is searchable by the service, but is not repeated in every index row.
type ReportIndex struct {
	Slug    string         `json:"slug"`
	Country string         `json:"country"`
	Date    string         `json:"date"`
	Title   string         `json:"title"`
	Summary string         `json:"summary,omitempty"`
	Place   string         `json:"place,omitempty"`
	Figures map[string]any `json:"figures,omitempty"`
	Names   []string       `json:"names,omitempty" yaml:"-"`
	CaseIDs []string       `json:"case_ids,omitempty" yaml:"-"`
	Digest  string         `json:"-" yaml:"-"`
	Body    string         `json:"-" yaml:"-"`
	Sources []struct {
		URL string `json:"url" yaml:"url"`
	} `json:"sources"`
}

// EventComparison contains public event facts, not private reasoning or raw sources.
// FactText must identify a quotation-checked fact in the proposed report.
type EventComparison struct {
	Report       string `json:"report"`
	Relation     string `json:"relation"`
	ExistingText string `json:"existing_text"`
	FactText     string `json:"fact_text"`
	Difference   string `json:"difference"`
}

type EventCandidate struct {
	Report  string   `json:"report"`
	Reasons []string `json:"reasons"`
	Score   float64  `json:"-"`
}

var identityStop = func() map[string]bool {
	out := map[string]bool{}
	for _, s := range strings.Fields("a an and are as at by cannabis marijuana marihuana ganja hashish hash hemp drug drugs police of on in over for from to the with after during more than about nearly allegedly alleged seized seizes seize arrest arrests arrested held jailed jail sentenced sentence sentencing kg kilograms kilogram g grams gram tonne tonnes t lb pounds million worth plants plant destroys destroyed destroy found accused man men woman women raid raids raided smuggling smuggled smuggles trafficking trafficker traffickers possession prison years year months month officers case court operation caught following amid says say said") {
		out[s] = true
	}
	return out
}()
var identityWords = regexp.MustCompile(`[\p{L}\p{N}]+`)
var identityNames = regexp.MustCompile(`[\p{Lu}][\p{L}’'-]{2,}`)
var identityCaseID = regexp.MustCompile(`(?i)\b(?:case|docket|fir|crime)\s*(?:no\.?|number|#)?\s*[:#]?\s*([a-z0-9][a-z0-9/-]{3,})`)

func IndexCaseIDs(text string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, match := range identityCaseID.FindAllStringSubmatch(text, -1) {
		id := strings.ToUpper(match[1])
		if strings.ContainsAny(id, "0123456789") && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func identityTokens(s string) map[string]bool {
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return unicode.ToLower(r)
	}, norm.NFKD.String(s))
	out := map[string]bool{}
	for _, w := range identityWords.FindAllString(s, -1) {
		if len([]rune(w)) > 1 && !identityStop[w] {
			out[w] = true
		}
	}
	return out
}

func IndexNames(text string) []string {
	seen := map[string]bool{}
	out := []string{}
	// Overlapping pairs matter: a preceding capitalized place or heading must
	// not consume the first word of a person's name ("Malaysia Parlan Dadeh").
	words := identityNames.FindAllStringIndex(text, -1)
	for i := 1; i < len(words); i++ {
		gap := text[words[i-1][1]:words[i][0]]
		if gap == "" || strings.TrimSpace(gap) != "" || strings.Contains(gap, "\n\n") {
			continue
		}
		name := text[words[i-1][0]:words[i-1][1]] + " " + text[words[i][0]:words[i][1]]
		if len(identityTokens(name)) >= 2 && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// Include country-level and state-level records across all dates. Court stages
// can be years apart, and country labels can change between accounts.
func RelatedCountries(a, b string) bool {
	return a != "" && b != "" && strings.Split(a, "/")[0] == strings.Split(b, "/")[0]
}

func identitySource(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	q := u.Query()
	for key := range q {
		k := strings.ToLower(key)
		if strings.HasPrefix(k, "utm_") || k == "fbclid" || k == "gclid" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	return u.Host + strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/amp"), "/") + "?" + u.RawQuery
}

// Similarity only nominates reports for a sourced comparison; it never merges
// records or proves that two events are identical. There is no top-N cutoff:
// every nominated match must be addressed before a new report can publish.
func EventCandidates(r Result, reports []ReportIndex) []EventCandidate {
	tokens := identityTokens(r.Title)
	frequencies := map[string]int{}
	for _, other := range reports {
		for word := range identityTokens(other.Title) {
			frequencies[word]++
		}
	}
	similarity := func(a, b map[string]bool) (float64, int) {
		left, right, shared, count := 0.0, 0.0, 0.0, 0
		for word := range a {
			w := math.Log(2 + float64(len(reports)+1)/float64(frequencies[word]+1))
			left += w
			if b[word] {
				shared += w
				count++
			}
		}
		for word := range b {
			right += math.Log(2 + float64(len(reports)+1)/float64(frequencies[word]+1))
		}
		return shared / math.Max(1, math.Max(left, right)), count
	}
	text := r.Title + " " + r.Summary
	for _, fact := range r.Facts {
		text += " " + fact.Text
	}
	names := identityTokens(strings.Join(IndexNames(text), " "))
	namePhrases := IndexNames(text)
	caseIDs := IndexCaseIDs(text)
	nameFrequency := map[string]int{}
	for _, other := range reports {
		for _, name := range other.Names {
			nameFrequency[strings.ToLower(name)]++
		}
	}
	sources := map[string]bool{}
	for _, source := range r.Sources {
		sources[identitySource(source.URL)] = true
	}
	figures := map[string]float64{}
	if r.Figures != nil {
		if r.Figures.KgSeized != nil {
			figures["kg_seized"] = *r.Figures.KgSeized
		}
		if r.Figures.PlantsDestroyed != nil {
			figures["plants_destroyed"] = float64(*r.Figures.PlantsDestroyed)
		}
		if r.Figures.Hectares != nil {
			figures["hectares"] = *r.Figures.Hectares
		}
	}
	out := []EventCandidate{}
	date, _ := time.Parse("2006-01-02", r.Date)
	for _, other := range reports {
		sharedSource := false
		for _, source := range other.Sources {
			if sources[identitySource(source.URL)] {
				sharedSource = true
			}
		}
		if !RelatedCountries(r.Country, other.Country) && !sharedSource && r.Country != "" {
			continue
		}
		reasons := []string{}
		score, count := similarity(tokens, identityTokens(other.Title))
		if sharedSource {
			reasons = append(reasons, "shared source (may be a roundup)")
		}
		for _, id := range caseIDs {
			for _, existingID := range other.CaseIDs {
				if id == existingID {
					reasons = append(reasons, "shared case identifier")
					break
				}
			}
		}
		if score >= .48 && count >= 2 {
			reasons = append(reasons, "distinctive title overlap")
		}
		equalFigure := false
		for key, value := range figures {
			if value > 0 && fmt.Sprint(other.Figures[key]) == fmt.Sprint(value) {
				equalFigure = true
			}
		}
		otherDate, _ := time.Parse("2006-01-02", other.Date)
		nearby := !date.IsZero() && !otherDate.IsZero() && math.Abs(date.Sub(otherDate).Hours()) <= 8*24
		placeScore, placeCount := similarity(identityTokens(r.Place), identityTokens(other.Place))
		if nearby && score >= .28 && (equalFigure || (placeCount > 0 && placeScore >= .65)) {
			reasons = append(reasons, "nearby date with matching place or quantity")
		}
		// A full rare name may occur only in the prose of the earlier case.
		otherNames := strings.ToLower(strings.Join(other.Names, " | "))
		for _, name := range namePhrases {
			if nameFrequency[strings.ToLower(name)] <= 6 && strings.Contains(otherNames, strings.ToLower(name)) && len(names) >= 2 {
				reasons = append(reasons, "shared uncommon name or case label; compare all case stages")
				break
			}
		}
		if len(reasons) > 0 {
			out = append(out, EventCandidate{Report: other.Slug, Reasons: reasons, Score: float64(len(reasons))*100 + score})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Report < out[j].Report
	})
	return out
}

func normalizedIdentityText(s string) string { return strings.Join(strings.Fields(s), " ") }

// CheckEventComparisons is a mandatory gate for new_event, including recovered
// old jobs. Supporting proposed facts must survive independent quotation checks.
func CheckEventComparisons(r Result, reports []ReportIndex, requireConfirmed bool) error {
	if r.Outcome != "new_event" {
		return nil
	}
	bySlug := map[string]ReportIndex{}
	for _, report := range reports {
		bySlug[report.Slug] = report
	}
	comparisons := map[string]EventComparison{}
	for _, c := range r.EventComparisons {
		if _, exists := comparisons[c.Report]; exists {
			return fmt.Errorf("duplicate event comparison for %s", c.Report)
		}
		if c.Relation != "distinct_event" && c.Relation != "campaign_component" {
			return fmt.Errorf("%s is not a separate event: use existing_case, additional_source or duplicate for a supported match", c.Report)
		}
		existing, exists := bySlug[c.Report]
		if !exists {
			return fmt.Errorf("event comparison target %s is no longer available", c.Report)
		}
		quote := normalizedIdentityText(c.ExistingText)
		if len(quote) < 20 || len(quote) > 2000 || !strings.Contains(normalizedIdentityText(existing.Body), quote) {
			return fmt.Errorf("event comparison for %s must quote an actual passage from its report body", c.Report)
		}
		factFound := false
		for _, fact := range r.Facts {
			if normalizedIdentityText(fact.Text) == normalizedIdentityText(c.FactText) && (!requireConfirmed || fact.confirmed) {
				factFound = true
			}
		}
		if !factFound {
			return fmt.Errorf("event comparison for %s needs a retained quotation-checked fact", c.Report)
		}
		if len(c.Difference) < 20 || len(c.Difference) > 600 || plain.MatchString(c.Difference) {
			return fmt.Errorf("event comparison for %s needs a short factual distinction", c.Report)
		}
		comparisons[c.Report] = c
	}
	for _, candidate := range EventCandidates(r, reports) {
		if _, ok := comparisons[candidate.Report]; !ok {
			return fmt.Errorf("missing event comparison for %s (%s); read its original report and compare the event and case, not just dates or titles", candidate.Report, strings.Join(candidate.Reasons, "; "))
		}
	}
	return nil
}
