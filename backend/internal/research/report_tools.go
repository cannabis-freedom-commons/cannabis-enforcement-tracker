package research

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// The research agent looks existing reports up through the tracker tool server instead of
// grepping a copy of the whole index and of every report: find_reports returns a short ranked
// list, with the comparisons the result check will require, and get_report one report's Markdown.
// Both read the job's own snapshot of the reports (dir/reports), the same one the check uses.

const findReportsDescription = "Find existing tracker reports that may record the same event or case, before you decide " +
	"between a new report and an update. Give what you know: country (tracker id), date (YYYY-MM-DD), title " +
	"(an English headline), place, keywords, names, case_ids, source_urls, kg_seized, plants_destroyed. The answer " +
	"lists first the reports the result check will require you to compare (the same matcher), then other likely " +
	"matches by place, words and date, each with slug, date, title, place, figures, tags and why it matched. " +
	"Call it again with other words or a wider window_days (default 45) when nothing fits. Read a report with get_report."

const getReportDescription = "Read one existing tracker report: its complete original Markdown (front matter, prose with " +
	"[@n] citations, sources). Pass the slug from find_reports. Read a report before you propose an update to it " +
	"or quote it in an event comparison."

var findReportsSchema = map[string]any{"type": "object", "properties": map[string]any{
	"country":          map[string]any{"type": "string", "description": "Tracker country id, e.g. br, us/ca"},
	"date":             map[string]any{"type": "string", "description": "Incident date, YYYY-MM-DD"},
	"window_days":      map[string]any{"type": "integer", "description": "How many days around the date to search (default 45, at most 1200; a later court stage can be years later)"},
	"title":            map[string]any{"type": "string", "description": "An English headline of the event"},
	"place":            map[string]any{"type": "string"},
	"keywords":         map[string]any{"type": "string", "description": "Other distinctive words: neighbourhood, agency, operation name"},
	"names":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	"case_ids":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	"source_urls":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	"kg_seized":        map[string]any{"type": "number"},
	"plants_destroyed": map[string]any{"type": "integer"},
	"limit":            map[string]any{"type": "integer", "description": "Other matches to list (default 10, at most 25)"},
}}

var getReportSchema = map[string]any{"type": "object", "properties": map[string]any{
	"slug": map[string]any{"type": "string"},
}, "required": []string{"slug"}}

type findReportsArgs struct {
	Country         string   `json:"country"`
	Date            string   `json:"date"`
	WindowDays      int      `json:"window_days"`
	Title           string   `json:"title"`
	Place           string   `json:"place"`
	Keywords        string   `json:"keywords"`
	Names           []string `json:"names"`
	CaseIDs         []string `json:"case_ids"`
	SourceURLs      []string `json:"source_urls"`
	KgSeized        *float64 `json:"kg_seized"`
	PlantsDestroyed *int     `json:"plants_destroyed"`
	Limit           int      `json:"limit"`
}

// reportLookup loads the snapshot's reports on first use (a run that never asks pays nothing).
type reportLookup struct {
	dir     string
	once    sync.Once
	reports []ReportIndex
	paths   map[string]string
	err     error
}

func newReportLookup(dir string) *reportLookup { return &reportLookup{dir: dir} }

func (l *reportLookup) load() error {
	l.once.Do(func() {
		l.reports, l.err = LoadReports(l.dir)
		l.paths = map[string]string{}
		_ = filepath.WalkDir(filepath.Join(l.dir, "reports"), func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".md") {
				l.paths[strings.TrimSuffix(d.Name(), ".md")] = path
			}
			return nil
		})
	})
	return l.err
}

func (l *reportLookup) get(raw json.RawMessage) string {
	var args struct {
		Slug string `json:"slug"`
	}
	if json.Unmarshal(raw, &args) != nil || strings.TrimSpace(args.Slug) == "" {
		return "Pass the report's slug, for example {\"slug\": \"cuiaba-lixeira-39-portions-arrest\"}."
	}
	if err := l.load(); err != nil {
		return "The reports could not be read: " + err.Error()
	}
	slug := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(strings.TrimSpace(args.Slug)), "existing-reports/"), ".md")
	path, ok := l.paths[slug]
	if !ok {
		return fmt.Sprintf("No report has the slug %q. Find it with find_reports.", slug)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "The report could not be read."
	}
	text := string(b)
	if len(text) > 60000 {
		text = text[:60000] + "\n[… cut at 60,000 characters]"
	}
	return text
}

func (l *reportLookup) find(raw json.RawMessage) string {
	var a findReportsArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return "The arguments were not understood: " + err.Error()
	}
	if err := l.load(); err != nil {
		return "The reports could not be read: " + err.Error()
	}
	a.Country = strings.ToLower(strings.TrimSpace(a.Country))
	if a.WindowDays <= 0 {
		a.WindowDays = 45
	}
	a.WindowDays = min(a.WindowDays, 1200)
	if a.Limit <= 0 {
		a.Limit = 10
	}
	a.Limit = min(a.Limit, 25)

	// the result check's own matcher, on what the agent knows so far
	probe := Result{Country: a.Country, Date: a.Date, Title: strings.TrimSpace(a.Title + " " + a.Keywords), Place: a.Place}
	text := strings.Join(a.Names, ", ")
	for _, id := range a.CaseIDs {
		text += " case no. " + id
	}
	probe.Facts = []Fact{{Text: text}}
	for _, u := range a.SourceURLs {
		probe.Sources = append(probe.Sources, Source{URL: u})
	}
	if a.KgSeized != nil || a.PlantsDestroyed != nil {
		probe.Figures = &Figures{KgSeized: a.KgSeized, PlantsDestroyed: a.PlantsDestroyed}
	}
	required := EventCandidates(probe, l.reports)
	if len(required) > 25 {
		required = required[:25]
	}
	listed := map[string]bool{}
	for _, c := range required {
		listed[c.Report] = true
	}

	// other likely matches: rare shared words of the title, place, keywords and names, near the date
	query := identityTokens(strings.Join([]string{a.Title, a.Place, a.Keywords, strings.Join(a.Names, " ")}, " "))
	frequency := map[string]int{}
	for _, r := range l.reports {
		for w := range identityTokens(r.Title + " " + r.Place) {
			frequency[w]++
		}
	}
	// words too common to tell events apart (a state's name, "de", the country) do not count
	common := len(l.reports)/60 + 5
	for w := range query {
		if frequency[w] > common || (len([]rune(w)) < 3 && !strings.ContainsAny(w, "0123456789")) {
			delete(query, w)
		}
	}
	date, _ := time.Parse("2006-01-02", a.Date)
	type hit struct {
		r       ReportIndex
		score   float64
		reasons []string
	}
	var hits []hit
	for _, r := range l.reports {
		if listed[r.Slug] || (a.Country != "" && !RelatedCountries(a.Country, strings.ToLower(r.Country))) {
			continue
		}
		var days float64
		if !date.IsZero() {
			other, err := time.Parse("2006-01-02", r.Date)
			if err != nil {
				continue
			}
			days = math.Abs(date.Sub(other).Hours() / 24)
			if days > float64(a.WindowDays) {
				continue
			}
		}
		words := identityTokens(r.Title + " " + r.Place + " " + r.Summary)
		score, shared := 0.0, []string{}
		for w := range query {
			if words[w] {
				score += math.Log(2 + float64(len(l.reports))/float64(frequency[w]+1))
				shared = append(shared, w)
			}
		}
		if len(query) > 0 && score == 0 {
			continue
		}
		sort.Strings(shared)
		reasons := []string{}
		if len(shared) > 0 {
			reasons = append(reasons, "shares: "+strings.Join(shared, ", "))
		}
		if !date.IsZero() {
			reasons = append(reasons, fmt.Sprintf("%d days apart", int(days)))
			score += 2 / (1 + days/7) // nearer first among equal words
		}
		hits = append(hits, hit{r, score, reasons})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].r.Slug < hits[j].r.Slug
	})

	bySlug := map[string]ReportIndex{}
	for _, r := range l.reports {
		bySlug[r.Slug] = r
	}
	var b strings.Builder
	if len(required) > 0 {
		b.WriteString("The result check will require a comparison with each of these (event_comparisons): read them with get_report.\n")
		for _, c := range required {
			b.WriteString(reportLine(bySlug[c.Report], c.Reasons))
		}
	} else {
		b.WriteString("The result check requires no comparison for these details.\n")
	}
	if len(hits) > a.Limit {
		hits = hits[:a.Limit]
	}
	if len(hits) > 0 {
		b.WriteString("\nOther possible matches, best first:\n")
		for _, h := range hits {
			b.WriteString(reportLine(h.r, h.reasons))
		}
	} else {
		within := ""
		if !date.IsZero() {
			within = fmt.Sprintf(" within %d days", a.WindowDays)
		}
		fmt.Fprintf(&b, "\nNo other report in %s%s shares these words.\n", or(a.Country, "any country"), within)
	}
	return b.String()
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// one compact line per report: what the agent needs to decide whether to open it
func reportLine(r ReportIndex, reasons []string) string {
	parts := []string{r.Slug, r.Date, r.Country}
	if r.Type != "" {
		parts = append(parts, r.Type)
	}
	parts = append(parts, r.Title)
	if r.Place != "" {
		parts = append(parts, "place: "+r.Place)
	}
	if len(r.Figures) > 0 {
		keys := make([]string, 0, len(r.Figures))
		for k := range r.Figures {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var figures []string
		for _, k := range keys {
			if v, ok := r.Figures[k]; ok && (k == "kg_seized" || k == "arrested" || k == "plants_destroyed" || k == "hectares" || k == "charged" || k == "sentenced" || k == "prison" || k == "deaths") {
				figures = append(figures, fmt.Sprintf("%s %v", k, v))
			}
		}
		if len(figures) > 0 {
			parts = append(parts, strings.Join(figures, ", "))
		}
	}
	if len(r.Tags) > 0 {
		parts = append(parts, "tags: "+strings.Join(r.Tags, " "))
	}
	if len(reasons) > 0 {
		parts = append(parts, "why: "+strings.Join(reasons, "; "))
	}
	return "- " + strings.Join(parts, " | ") + "\n"
}
