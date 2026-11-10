package research

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Details are a report's structured facts beyond its figures: who acted, what was taken, where,
// and how far the case went. Every value needs a supporting quotation (DataQuote), checked like
// the facts' quotations; an unsupported value is left out with an open question.
type Details struct {
	Agencies      Terms     `json:"agencies,omitempty"`     // registry ids (src/data/agencies.json) or names as published
	AgencyTypes   Terms     `json:"agency_types,omitempty"` // vocabulary agency_type
	Products      []Product `json:"products,omitempty"`
	Activity      Terms     `json:"activity,omitempty"`
	Setting       Terms     `json:"setting,omitempty"`
	Environment   Terms     `json:"environment,omitempty"`
	Charges       Terms     `json:"charges,omitempty"` // as stated: "cultivation without a licence"
	CaseStage     string    `json:"case_stage,omitempty"`
	SentenceTypes Terms     `json:"sentence_types,omitempty"`
	Force         Terms     `json:"force,omitempty"`
	Period        *Period   `json:"period,omitempty"`
	DatePrecision string    `json:"date_precision,omitempty"`
	Region        string    `json:"region,omitempty"` // the state, province or district, in English
	Route         *Route    `json:"route,omitempty"`
	PartOf        string    `json:"part_of,omitempty"` // a campaign report this event is a component of
	Related       Terms     `json:"related,omitempty"` // other stages or parts of the same case
}

// Terms is a list of terms that also accepts a single term: the model often writes force: "lethal"
// where the format takes [lethal], and a parse error rejected the whole answer for it.
type Terms []string

func (t *Terms) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		if one == "" {
			*t = nil
		} else {
			*t = Terms{one}
		}
		return nil
	}
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	*t = list
	return nil
}

type Product struct {
	Product string   `json:"product"`
	Kg      *float64 `json:"kg,omitempty"`
	Plants  *int     `json:"plants,omitempty"`
}

type Period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type Route struct {
	From     []string `json:"from,omitempty"` // country ids
	To       []string `json:"to,omitempty"`
	Crossing string   `json:"crossing,omitempty"`
}

// DataQuote supports one structured value: Field is "figures.<key>" or "details.<key>".
type DataQuote struct {
	Field     string `json:"field"`
	Source    int    `json:"source"`
	Quote     string `json:"quote"`
	confirmed bool
}

// Vocabularies are the allowed values of the structured fields (src/data/vocabularies.json) and
// the agency registry (src/data/agencies.json).
type Vocabularies struct {
	Terms    map[string]map[string]bool
	Agencies map[string]bool
}

func LoadVocabularies(repo string) (Vocabularies, error) {
	paths := []string{filepath.Join(repo, "src/data/vocabularies.json"), filepath.Join(repo, "src/data/agencies.json")}
	return cachedLoad("vocabularies", paths, func() (Vocabularies, error) { return readVocabularies(repo) })
}

func readVocabularies(repo string) (Vocabularies, error) {
	v := Vocabularies{Terms: map[string]map[string]bool{}, Agencies: map[string]bool{}}
	b, err := os.ReadFile(filepath.Join(repo, "src/data/vocabularies.json"))
	if err != nil {
		return v, err
	}
	var lists map[string][]struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(b, &lists); err != nil {
		return v, err
	}
	for name, terms := range lists {
		v.Terms[name] = map[string]bool{}
		for _, t := range terms {
			v.Terms[name][t.ID] = true
		}
	}
	b, err = os.ReadFile(filepath.Join(repo, "src/data/agencies.json"))
	if err != nil {
		return v, err
	}
	var agencies []struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(b, &agencies); err != nil {
		return v, err
	}
	for _, a := range agencies {
		v.Agencies[a.ID] = true
	}
	return v, nil
}

var agencyName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} .,'’&()/-]{1,99}$`)

func (v Vocabularies) term(list, id string) error {
	if !v.Terms[list][id] {
		return fmt.Errorf("details: %q is not a %s (see src/data/vocabularies.json)", id, list)
	}
	return nil
}

// Validate checks the details against the vocabularies and the country list.
func (d *Details) Validate(v Vocabularies, countries map[string]bool) error {
	if d == nil {
		return nil
	}
	lists := []struct {
		name   string
		values []string
	}{{"agency_type", d.AgencyTypes}, {"activity", d.Activity}, {"setting", d.Setting}, {"environment", d.Environment}, {"sentence_type", d.SentenceTypes}, {"force", d.Force}}
	for _, l := range lists {
		if len(l.values) > 10 {
			return fmt.Errorf("details: too many %s values", l.name)
		}
		seen := map[string]bool{}
		for _, id := range l.values {
			if err := v.term(l.name, id); err != nil {
				return err
			}
			if seen[id] {
				return fmt.Errorf("details: %s repeats %q", l.name, id)
			}
			seen[id] = true
		}
	}
	if len(d.Agencies) > 10 {
		return errors.New("details: too many agencies")
	}
	for _, a := range d.Agencies {
		if !v.Agencies[a] && (!agencyName.MatchString(a) || slug.MatchString(a)) {
			return fmt.Errorf("details: agency %q is neither a registry id (src/data/agencies.json) nor an agency name as published", a)
		}
	}
	if len(d.Products) > 10 {
		return errors.New("details: too many products")
	}
	for _, p := range d.Products {
		if err := v.term("product", p.Product); err != nil {
			return err
		}
		if (p.Kg != nil && *p.Kg < 0) || (p.Plants != nil && *p.Plants < 0) {
			return errors.New("details: a product quantity is negative")
		}
	}
	if len(d.Charges) > 10 {
		return errors.New("details: too many charges")
	}
	for _, c := range d.Charges {
		if len(c) < 3 || len(c) > 160 || plain.MatchString(c) {
			return errors.New("details: a charge must be short plain text as stated")
		}
	}
	if d.CaseStage != "" {
		if err := v.term("case_stage", d.CaseStage); err != nil {
			return err
		}
	}
	if d.DatePrecision != "" {
		if err := v.term("date_precision", d.DatePrecision); err != nil {
			return err
		}
	}
	if d.Period != nil {
		start, err1 := time.Parse("2006-01-02", d.Period.Start)
		end, err2 := time.Parse("2006-01-02", d.Period.End)
		if err1 != nil || err2 != nil || end.Before(start) {
			return errors.New("details: period needs start and end dates (YYYY-MM-DD), end not before start")
		}
	}
	if len(d.Region) > 100 || plain.MatchString(d.Region) {
		return errors.New("details: invalid region")
	}
	if d.Route != nil {
		for _, c := range append(append([]string{}, d.Route.From...), d.Route.To...) {
			if !countries[c] {
				return fmt.Errorf("details: route country %q is not a country id", c)
			}
		}
		if len(d.Route.Crossing) > 100 || plain.MatchString(d.Route.Crossing) {
			return errors.New("details: invalid route crossing")
		}
	}
	for _, s := range append([]string{d.PartOf}, d.Related...) {
		if s != "" && !slug.MatchString(s) {
			return errors.New("details: part_of and related take report slugs")
		}
	}
	if len(d.Related) > 10 {
		return errors.New("details: too many related reports")
	}
	return nil
}

// dataFields lists the structured values a result gives, as "figures.<key>"/"details.<key>".
// part_of, related, date_precision and region are judgments about the record, not quoted claims.
func (r Result) dataFields() []string {
	var out []string
	add := func(prefix string, v any) {
		b, _ := json.Marshal(v)
		var m map[string]json.RawMessage
		if json.Unmarshal(b, &m) != nil {
			return
		}
		for k := range m {
			out = append(out, prefix+k)
		}
	}
	if r.Figures != nil {
		add("figures.", r.Figures)
	}
	if r.Details != nil {
		add("details.", r.Details)
	}
	sort.Strings(out)
	kept := out[:0]
	for _, f := range out {
		if !unquotedField[f] {
			kept = append(kept, f)
		}
	}
	return kept
}

var unquotedField = map[string]bool{"details.part_of": true, "details.related": true, "details.date_precision": true, "details.region": true}

func validateDataQuotes(r Result) error {
	if len(r.DataQuotes) > 80 {
		return errors.New("too many data quotes")
	}
	for _, q := range r.DataQuotes {
		if !strings.HasPrefix(q.Field, "figures.") && !strings.HasPrefix(q.Field, "details.") {
			return fmt.Errorf("data quote field %q must be figures.<key> or details.<key>", q.Field)
		}
		if q.Source < 1 || q.Source > len(r.Sources) {
			return fmt.Errorf("data quote for %s cites a source that does not exist", q.Field)
		}
		if len(strings.TrimSpace(q.Quote)) < 3 || len(q.Quote) > 1000 {
			return fmt.Errorf("data quote for %s needs a short verbatim quotation", q.Field)
		}
	}
	return nil
}

// MissingQuotes names the structured values that have no quotation at all.
func (r Result) MissingQuotes() []string {
	quoted := map[string]bool{}
	for _, q := range r.DataQuotes {
		quoted[q.Field] = true
	}
	var missing []string
	for _, f := range r.dataFields() {
		if !quoted[f] {
			missing = append(missing, f)
		}
	}
	return missing
}

// confirmData checks each data quote against the opened sources (nil: not opened) and removes
// structured values that no quote supports, noting why in the uncertainties. A value whose
// quoted sources could not be opened again stays, as the AI read it. It records which sources
// support each kept value.
func (r *Result) confirmData(opened []*supportCheck) []string {
	var left []string
	support := map[string][]int{}
	unchecked := map[string][]int{}
	failed := map[string]string{}
	for i := range r.DataQuotes {
		q := &r.DataQuotes[i]
		if q.Source < 1 || q.Source > len(opened) {
			continue
		}
		check := opened[q.Source-1]
		if check == nil {
			unchecked[q.Field] = append(unchecked[q.Field], q.Source)
			continue
		}
		if check.supports(q.Quote) {
			q.confirmed = true
			support[q.Field] = append(support[q.Field], q.Source)
		} else {
			failed[q.Field] = check.publisher
		}
	}
	r.DataSources = map[string][]int{}
	for _, field := range r.dataFields() {
		switch {
		case len(support[field]) > 0:
			r.DataSources[field] = uniqueInts(support[field])
		case len(unchecked[field]) > 0:
			r.DataSources[field] = uniqueInts(unchecked[field])
		default:
			why := "no supporting quotation was given"
			if publisher, ok := failed[field]; ok {
				why = "its quotation was not found in " + publisher
			}
			r.dropField(field)
			left = append(left, field)
			r.Uncertainties = note(r.Uncertainties, fmt.Sprintf("Left out %s, as %s.", fieldLabel(field), why))
		}
	}
	if len(r.DataSources) == 0 {
		r.DataSources = nil
	}
	return left
}

type supportCheck struct {
	supports  func(string) bool
	publisher string
}

func uniqueInts(v []int) []int {
	sort.Ints(v)
	out := v[:0]
	for i, n := range v {
		if i == 0 || n != v[i-1] {
			out = append(out, n)
		}
	}
	return out
}

func fieldLabel(field string) string {
	return strings.ReplaceAll(strings.SplitN(field, ".", 2)[1], "_", " ")
}

// dropField removes one structured value by its path.
func (r *Result) dropField(field string) {
	prefix, key, _ := strings.Cut(field, ".")
	var target any
	switch prefix {
	case "figures":
		target = r.Figures
	case "details":
		target = r.Details
	}
	if target == nil {
		return
	}
	b, _ := json.Marshal(target)
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return
	}
	delete(m, key)
	b, _ = json.Marshal(m)
	switch prefix {
	case "figures":
		f := &Figures{}
		_ = json.Unmarshal(b, f)
		r.Figures = f
		if len(m) == 0 {
			r.Figures = nil
		}
	case "details":
		d := &Details{}
		_ = json.Unmarshal(b, d)
		r.Details = d
		if len(m) == 0 {
			r.Details = nil
		}
	}
}

// renumberData follows the sources' new numbers after uncited ones are left out. A value whose
// only support was a source no fact cites is left out too: a report lists the sources its text cites.
func (r *Result) renumberData(renumber map[int]int) []string {
	kept := r.DataQuotes[:0]
	for _, q := range r.DataQuotes {
		if n := renumber[q.Source]; n != 0 {
			q.Source = n
			kept = append(kept, q)
		}
	}
	r.DataQuotes = kept
	var left []string
	for _, field := range r.dataFields() {
		var out []int
		for _, s := range r.DataSources[field] {
			if n := renumber[s]; n != 0 {
				out = append(out, n)
			}
		}
		if len(out) > 0 {
			r.DataSources[field] = uniqueInts(out)
			continue
		}
		if r.DataSources != nil {
			delete(r.DataSources, field)
		}
		r.dropField(field)
		left = append(left, field)
		r.Uncertainties = note(r.Uncertainties, fmt.Sprintf("Left out %s, as its only supporting source is not cited by the report's text.", fieldLabel(field)))
	}
	if len(r.DataSources) == 0 {
		r.DataSources = nil
	}
	return left
}

// detailsNode writes the details in schema order.
func (d *Details) node() *yaml.Node {
	if d == nil {
		return nil
	}
	m := &yaml.Node{Kind: yaml.MappingNode}
	list := func(k string, v []string) {
		if len(v) > 0 {
			m.Content = append(m.Content, str(k), flow(strs(v)...))
		}
	}
	text := func(k, v string) {
		if v != "" {
			m.Content = append(m.Content, str(k), str(v))
		}
	}
	list("agencies", d.Agencies)
	list("agency_types", d.AgencyTypes)
	if len(d.Products) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, p := range d.Products {
			item := flow(str("product"), str(p.Product))
			item.Kind = yaml.MappingNode
			if p.Kg != nil {
				item.Content = append(item.Content, str("kg"), num(*p.Kg))
			}
			if p.Plants != nil {
				item.Content = append(item.Content, str("plants"), integer(*p.Plants))
			}
			seq.Content = append(seq.Content, item)
		}
		m.Content = append(m.Content, str("products"), seq)
	}
	list("activity", d.Activity)
	list("setting", d.Setting)
	list("environment", d.Environment)
	list("charges", d.Charges)
	text("case_stage", d.CaseStage)
	list("sentence_types", d.SentenceTypes)
	list("force", d.Force)
	if d.Period != nil {
		p := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle, Content: []*yaml.Node{
			str("start"), {Kind: yaml.ScalarNode, Tag: "!!timestamp", Value: d.Period.Start},
			str("end"), {Kind: yaml.ScalarNode, Tag: "!!timestamp", Value: d.Period.End}}}
		m.Content = append(m.Content, str("period"), p)
	}
	text("date_precision", d.DatePrecision)
	text("region", d.Region)
	if d.Route != nil && (len(d.Route.From)+len(d.Route.To) > 0 || d.Route.Crossing != "") {
		route := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
		if len(d.Route.From) > 0 {
			route.Content = append(route.Content, str("from"), flow(strs(d.Route.From)...))
		}
		if len(d.Route.To) > 0 {
			route.Content = append(route.Content, str("to"), flow(strs(d.Route.To)...))
		}
		if d.Route.Crossing != "" {
			route.Content = append(route.Content, str("crossing"), str(d.Route.Crossing))
		}
		m.Content = append(m.Content, str("route"), route)
	}
	text("part_of", d.PartOf)
	list("related", d.Related)
	if len(m.Content) == 0 {
		return nil
	}
	return m
}

// dataSourcesNode writes which sources support each structured value.
func dataSourcesNode(sources map[string][]int) *yaml.Node {
	if len(sources) == 0 {
		return nil
	}
	keys := make([]string, 0, len(sources))
	for k := range sources {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	m := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range keys {
		var items []*yaml.Node
		for _, n := range sources[k] {
			items = append(items, integer(n))
		}
		m.Content = append(m.Content, str(k), flow(items...))
	}
	return m
}

// StructuredInstructions is the structured-data contract, appended to the agent's instructions by
// the runner so that it always matches this service's parser.
//
//go:embed structured_prompt.md
var StructuredInstructions string

// structuredSection renders StructuredInstructions with the current vocabularies.
func structuredSection(repo string) (string, error) {
	b, err := os.ReadFile(filepath.Join(repo, "src/data/vocabularies.json"))
	if os.IsNotExist(err) {
		return "", nil // a checkout from before the structured data
	}
	if err != nil {
		return "", err
	}
	var lists map[string][]struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err = json.Unmarshal(b, &lists); err != nil {
		return "", err
	}
	out := StructuredInstructions
	for name, terms := range lists {
		var parts []string
		for _, t := range terms {
			parts = append(parts, t.ID+" ("+strings.ToLower(t.Description)+")")
		}
		out = strings.ReplaceAll(out, "{"+name+"}", strings.Join(parts, "; ")+".")
	}
	b, err = os.ReadFile(filepath.Join(repo, "src/data/agencies.json"))
	if err != nil {
		return "", err
	}
	var agencies []struct {
		ID, Name string
	}
	if err = json.Unmarshal(b, &agencies); err != nil {
		return "", err
	}
	var ids []string
	for _, a := range agencies {
		ids = append(ids, a.ID+" = "+a.Name)
	}
	out = strings.ReplaceAll(out, "{AGENCIES}", strings.Join(ids, "; "))
	if strings.Contains(out, "{") && regexp.MustCompile(`\{[a-z_A-Z]+\}`).MatchString(out) {
		return "", errors.New("structured instructions name an unknown vocabulary")
	}
	return out, nil
}
