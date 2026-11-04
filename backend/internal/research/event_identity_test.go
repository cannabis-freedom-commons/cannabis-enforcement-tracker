package research

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
)

func identityIndex(slug, country, date, title, body string) ReportIndex {
	return ReportIndex{Slug: slug, Country: country, Date: date, Title: title, Body: body, Names: IndexNames(title + " " + body), CaseIDs: IndexCaseIDs(body)}
}

func TestEventCandidatesFindAuditedCasePatterns(t *testing.T) {
	// Regression pairs from the report audit: renamed places and quantities,
	// and later court stages whose original incident dates are years apart.
	for _, tc := range []struct {
		name     string
		old      ReportIndex
		proposed Result
	}{
		{"court stage", identityIndex("parlan-original", "my", "2003-08-01", "Penang cannabis conviction", "Parlan Dadeh was convicted of cannabis trafficking."), Result{Country: "my", Date: "2006-02-01", Title: "Appeal dismissed in Malaysia", Facts: []Fact{{Text: "Parlan Dadeh lost his appeal."}}}},
		{"renamed place", identityIndex("bengaluru-original", "in", "2026-10-03", "Bengaluru Kambipura Innova 317kg", "Officers intercepted the Innova near Kambipura."), Result{Country: "in", Date: "2026-10-03", Title: "Bengaluru Kumbalagodu 317kg arrests"}},
		{"body name across states", identityIndex("angelos-original", "us/ut", "2004-11-16", "Salt Lake City cannabis sentence", "Weldon Angelos was sentenced to 55 years in prison."), Result{Country: "us", Date: "2016-06-03", Title: "Federal prisoner released", Facts: []Fact{{Text: "Weldon Angelos was released from prison."}}}},
		{"case identifier", identityIndex("case-original", "in", "2020-01-01", "Warehouse search", "Police registered case no. CR-425/2020 following the search."), Result{Country: "in", Date: "2026-11-01", Title: "Trafficking verdict", Facts: []Fact{{Text: "The verdict concerns case CR-425/2020."}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := EventCandidates(tc.proposed, []ReportIndex{tc.old})
			if len(got) != 1 || got[0].Report != tc.old.Slug {
				t.Fatalf("missed audited pattern: %+v", got)
			}
		})
	}
}

func TestEventCandidatesDoNotDecideRoundupIdentity(t *testing.T) {
	old := identityIndex("different-seizure", "hk", "2026-10-01", "Airport cannabis seizure", "Customs arrested a 22-year-old passenger with 5.1 kilograms.")
	old.Sources = append(old.Sources, struct {
		URL string `json:"url" yaml:"url"`
	}{URL: "https://www.example.org/roundup/?utm_source=feed"})
	r := valid()
	r.Country, r.Title = "hk", "Airport cannabis seizure"
	r.Facts[0].Text = "Customs arrested a 39-year-old passenger with nine kilograms."
	r.Sources[0].URL = "https://example.org/roundup/amp"
	if got := EventCandidates(r, []ReportIndex{old}); len(got) != 1 {
		t.Fatal(got)
	}
	r.EventComparisons = []EventComparison{{Report: old.Slug, Relation: "distinct_event", ExistingText: old.Body, FactText: r.Facts[0].Text, Difference: "The passengers have different ages and carried different quantities on separate journeys."}}
	if err := CheckEventComparisons(r, []ReportIndex{old}, false); err != nil {
		t.Fatal("separate supported incidents cannot pass", err)
	}
	r.EventComparisons[0].Relation = "campaign_component"
	if err := CheckEventComparisons(r, []ReportIndex{old}, false); err != nil {
		t.Fatal("campaign component cannot pass", err)
	}
	r.EventComparisons[0].Relation = "same_case"
	if err := CheckEventComparisons(r, []ReportIndex{old}, false); err == nil {
		t.Fatal("same case accepted as new")
	}
}

func TestEventComparisonRequiresRetainedConfirmedFactsAndOriginalPassage(t *testing.T) {
	r := valid()
	old := identityIndex("older-raid", "th", "2025-01-01", r.Title, "Officers searched a different farm in Chiang Mai on Monday.")
	r.Title = "Chiang Mai farm search"
	// Force a candidate without depending on generic words in the title.
	old.Title = r.Title
	comparison := EventComparison{Report: old.Slug, Relation: "distinct_event", ExistingText: old.Body, FactText: r.Facts[0].Text, Difference: "The older report describes a different farm searched the previous year."}
	if err := CheckEventComparisons(r, []ReportIndex{old}, false); err == nil {
		t.Fatal("missing comparison accepted")
	}
	r.EventComparisons = []EventComparison{comparison}
	if err := CheckEventComparisons(r, []ReportIndex{old}, true); err == nil {
		t.Fatal("unchecked fact accepted")
	}
	confirmed, _, _, err := r.Confirm(context.Background(), safeurl.Evidence{URL: r.Sources[0].URL, Content: r.Facts[0].Quote}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckEventComparisons(confirmed, []ReportIndex{old}, true); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Result){
		func(r *Result) { r.EventComparisons[0].ExistingText = "An invented passage that nobody reported." },
		func(r *Result) { r.EventComparisons[0].FactText = "An invented fact that has no checked quotation." },
		func(r *Result) { r.Facts = nil },
		func(r *Result) { r.EventComparisons = append(r.EventComparisons, comparison) },
	} {
		copy := confirmed
		copy.EventComparisons = append([]EventComparison(nil), confirmed.EventComparisons...)
		mutate(&copy)
		if err := CheckEventComparisons(copy, []ReportIndex{old}, true); err == nil {
			t.Fatal("invalid comparison accepted", copy)
		}
	}
}

func TestUnrelatedCountryAndGenericArrestCountsDoNotNominateMatches(t *testing.T) {
	old := identityIndex("unrelated", "gb", "2026-01-02", "Police arrest two men", "Police arrested two men at a cannabis farm.")
	r := valid()
	r.Title = old.Title
	if got := EventCandidates(r, []ReportIndex{old}); len(got) != 0 {
		t.Fatal(got)
	}
	old.Country = r.Country
	old.Figures = map[string]any{"arrested": 2}
	two := 2
	r.Figures = &Figures{Arrested: &two}
	if got := EventCandidates(r, []ReportIndex{old}); len(got) != 0 {
		t.Fatal("generic count became identity proof", got)
	}
}

func TestRetiredReportAliasesAreReservedAndStrictlyParsed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "src/lib/report-aliases.js")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("// comment\nexport default {\"slugs\":{\"old-case\":\"live-case\"},\"codes\":{\"ABC123\":\"live-case\"}};\n"), 0644)
	aliases, digest, err := ReadReportAliases(dir)
	if err != nil || aliases.Slugs["old-case"] != "live-case" || digest == "" {
		t.Fatal(aliases, digest, err)
	}
	for _, pair := range [][2]string{{"old-case", ""}, {"new-case", "ABC123"}} {
		if err := CheckRetiredIdentity(dir, pair[0], pair[1]); err == nil {
			t.Fatal("retired identity reusable")
		}
	}
	if err := CheckRetiredIdentity(dir, "new-case", "XYZ123"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("export default (() => { throw 'must not run' })();"), 0644)
	if _, _, err := ReadReportAliases(dir); err == nil {
		t.Fatal("executable module accepted")
	}
}

func TestIdentityIndexKeepsSearchMetadataWithoutRepeatingBody(t *testing.T) {
	r := identityIndex("original", "my", "2003-01-01", "Conviction", "Parlan Dadeh was convicted in case CR-425/2020.")
	b, err := json.Marshal(r)
	if err != nil || !strings.Contains(string(b), "Parlan Dadeh") || !strings.Contains(string(b), "CR-425/2020") || strings.Contains(string(b), "was convicted") {
		t.Fatal(string(b), err)
	}
}
