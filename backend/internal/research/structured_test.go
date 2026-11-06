package research

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
)

func intp(n int) *int           { return &n }
func floatp(n float64) *float64 { return &n }

func structuredResult() Result {
	r := valid()
	r.Facts = []Fact{{Text: "Police raided the farm.", Source: 1, Quote: "Police raided the farm in the province."}}
	r.Figures = &Figures{Arrested: intp(2), KgSeized: floatp(40), Convicted: intp(1)}
	r.Details = &Details{Agencies: []string{"th-oncb"}, Products: []Product{{Product: "flower", Kg: floatp(40)}}, Setting: []string{"farm"}, CaseStage: "charged", DatePrecision: "exact", PartOf: "national-campaign"}
	r.DataQuotes = []DataQuote{
		{Field: "figures.arrested", Source: 1, Quote: "Two men were arrested"},
		{Field: "figures.kg_seized", Source: 1, Quote: "40 kilograms of marijuana"},
		{Field: "figures.convicted", Source: 1, Quote: "a sentence that is nowhere"},
		{Field: "details.agencies", Source: 1, Quote: "the ONCB said"},
		{Field: "details.products", Source: 1, Quote: "40 kilograms of marijuana"},
		{Field: "details.setting", Source: 1, Quote: "Police raided the farm"},
	}
	return r
}

func TestStructuredValuesNeedQuotations(t *testing.T) {
	vocab, err := LoadVocabularies(repo)
	if err != nil {
		t.Fatal(err)
	}
	r := structuredResult()
	b, _ := json.Marshal(r)
	parsed, err := Strict(b)
	if err != nil {
		t.Fatal(err)
	}
	parsed.CreatedByModel = r.CreatedByModel
	if err := parsed.Validate(map[string]bool{"th": true}, vocab); err != nil {
		t.Fatal(err)
	}
	if missing := strings.Join(parsed.MissingQuotes(), ","); missing != "details.case_stage" {
		t.Fatal("missing quotes", missing)
	}
	for _, mutate := range []func(*Result){
		func(r *Result) { r.Details.Setting = []string{"spaceship"} },
		func(r *Result) { r.Details.Agencies = []string{"made-up-id"} },
		func(r *Result) { r.Details.Products[0].Product = "cocaine" },
		func(r *Result) { r.DataQuotes[0].Source = 3 },
		func(r *Result) { r.DataQuotes[0].Field = "title" },
		func(r *Result) { r.Details.Period = &Period{Start: "2026-02-02", End: "2026-01-01"} },
	} {
		v := structuredResult()
		mutate(&v)
		if v.Validate(map[string]bool{"th": true}, vocab) == nil {
			t.Fatalf("accepted %+v", v.Details)
		}
	}
	page := "Police raided the farm in the province. Two men were arrested; 40 kilograms of marijuana were found, the ONCB said."
	got, _, check, err := r.Confirm(context.Background(), safeurl.Evidence{URL: r.Sources[0].URL, Content: page}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Figures.Convicted != nil || got.Details.CaseStage != "" || got.Figures.Arrested == nil || got.Details.Agencies[0] != "th-oncb" || got.Details.PartOf != "national-campaign" {
		t.Fatalf("figures %+v details %+v", got.Figures, got.Details)
	}
	if strings.Join(check.DataLeftOut, ",") != "details.case_stage,figures.convicted" {
		t.Fatal(check.DataLeftOut)
	}
	if len(got.DataSources) != 5 || got.DataSources["figures.kg_seized"][0] != 1 {
		t.Fatal(got.DataSources)
	}
	notes := strings.Join(got.Uncertainties, " | ")
	if !strings.Contains(notes, "convicted, as its quotation was not found in Publisher") || !strings.Contains(notes, "case stage, as no supporting quotation was given") {
		t.Fatal(notes)
	}
	// the written report passes the report check, details and data_sources included
	md := got.Markdown() + "\nThe police said the farm had been under watch for weeks before officers moved in, and that the harvest was destined for buyers in the city, according to a statement the provincial police released on the day of the raid. [@1]\n"
	if !strings.Contains(md, "details:") || !strings.Contains(md, "data_sources:") || !strings.Contains(md, "figures.arrested: [1]") {
		t.Fatal(md)
	}
	if err := CheckReport(repo, "reports/2026/01/example-raid.md", md); err != nil {
		t.Fatal(err, "\n", md)
	}
	broken := strings.Replace(md, "figures.arrested: [1]", "figures.arrested: [4]", 1)
	if err := CheckReport(repo, "reports/2026/01/example-raid.md", broken); err == nil {
		t.Fatal("accepted a data source the report does not list")
	}
}

func TestStructuredPromptNamesEveryVocabulary(t *testing.T) {
	text, err := structuredSection(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"th-oncb = Office of the Narcotics Control Board", "hashish (resin and pressed resin", "case_stage", "data_quotes"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing", want)
		}
	}
}
