package research

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
)

const existingReport = `---
code: 7KQ2MX
date: 2026-10-30
country: ua
coordinates: [48.1765, 23.2912]
title: Khust court jails a man for five years
summary: >-
  The Khust District Court sentenced a man
  to five years.
status: verified
sources:
  - title: First story
    url: https://first.example/story
    publisher: First
    date: 2026-10-30
    language: uk
---

The Khust District Court sentenced a man to five years in prison. [@1] Police arrested him in July. [@1]
`

func writeReport(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "reports/2026/10"), 0755)
	os.WriteFile(filepath.Join(dir, "reports/2026/10/khust-five-years.md"), []byte(existingReport), 0644)
	return dir
}

func TestUpdateReportAddsSourceAndKeepsTheRest(t *testing.T) {
	dir := writeReport(t)
	sources := []Source{{URL: "https://ua.news/en/story", Title: "Man sentenced", Publisher: "UA.News", Date: "2026-10-30"}}
	u := ReportUpdate{Report: "khust-five-years", Facts: []Fact{
		{Text: "The Khust District Court sentenced a man to five years in prison.", Source: 1, Quote: "sentenced to five years in prison"},
		{Text: "He had grown about 240 plants in a greenhouse.", NewDetail: true, Source: 1, Quote: "about 240 plants in a greenhouse"},
	}, Uncertainties: []string{"UA.News does not name the court's judge."}}
	for i := range u.Facts {
		u.Facts[i].confirmed = true
	}
	d, err := UpdateReport(dir, u, sources, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"date: 2026-10-30\n", "coordinates: [48.1765, 23.2912]", "summary: >-", "status: verified", "url: https://ua.news/en/story", "five years in prison. [@1,2] Police arrested him in July. [@1]", "He had grown about 240 plants in a greenhouse. [@2]", "UA.News does not name"} {
		if !strings.Contains(d.Content, want) {
			t.Fatalf("missing %q in\n%s", want, d.Content)
		}
	}
	if d.Path != "reports/2026/10/khust-five-years.md" || d.PreviousPath != d.Path || d.Title != "Khust court jails a man for five years" {
		t.Fatal(d.Path, d.Title)
	}
	os.WriteFile(filepath.Join(dir, d.Path), []byte(d.Content), 0644)
	if _, err = UpdateReport(dir, u, sources, nil); !errors.Is(err, ErrUnchanged) {
		t.Fatal("repeated update changed the report", err)
	}
	u.Summary = "A corrected summary."
	if d, err = UpdateReport(dir, u, sources, nil); err != nil || !strings.Contains(d.Content, "status: reported") || !strings.Contains(d.Content, "A corrected summary.") {
		t.Fatal(err, d.Content)
	}
	if _, err = UpdateReport(dir, ReportUpdate{Report: "missing-report", Facts: u.Facts}, sources, nil); err == nil {
		t.Fatal("missing report accepted")
	}
}

func TestAdditionalSourceAndReportUpdatesValidate(t *testing.T) {
	countries := map[string]bool{"th": true}
	r := Result{Outcome: "additional_source", ExistingReport: "example-raid", Explanation: "Another account of a recorded raid.", Sources: valid().Sources, Facts: valid().Facts}
	if err := r.Validate(countries); err != nil {
		t.Fatal(err)
	}
	r.ExistingReport = ""
	if r.Validate(countries) == nil {
		t.Fatal("additional source without its report accepted")
	}
	n := valid()
	n.Updates = []ReportUpdate{{Report: "other-raid", Facts: []Fact{{Text: "Police raided the farm.", Source: 1, Quote: "Police raided the farm in the province."}}}}
	if err := n.Validate(countries); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Result){
		func(r *Result) { r.Updates[0].Report = "example-raid" },
		func(r *Result) { r.Updates[0].Facts[0].Source = 2 },
		func(r *Result) { r.Updates[0].Summary = "<b>x</b>" },
		func(r *Result) { r.Outcome = "not_relevant" },
	} {
		v := valid()
		v.Updates = []ReportUpdate{{Report: "other-raid", Facts: []Fact{{Text: "Police raided the farm.", Source: 1, Quote: "Police raided the farm in the province."}}}}
		mutate(&v)
		if v.Validate(countries) == nil {
			t.Fatal("invalid update accepted", v.Updates, v.Outcome)
		}
	}
}

func TestAdditionalSourceCanCorroborateMoreThanTenFacts(t *testing.T) {
	r := valid()
	r.Outcome, r.ExistingReport = "additional_source", r.Slug
	for len(r.Facts) < 30 {
		r.Facts = append(r.Facts, r.Facts[0])
	}
	if err := r.Validate(map[string]bool{"th": true}); err != nil {
		t.Fatal(err)
	}
	r.Facts = append(r.Facts, r.Facts[0])
	if r.Validate(map[string]bool{"th": true}) == nil {
		t.Fatal("unbounded corroborating facts")
	}
}

func TestConfirmChecksAndRenumbersUpdateFacts(t *testing.T) {
	r := valid()
	r.Sources = append(r.Sources, Source{URL: "https://news.example/b", Title: "Second", Publisher: "Second"})
	r.Facts = append(r.Facts, Fact{Text: "Officers seized plants.", Source: 2, Quote: "officers seized plants at the farm"})
	r.Facts[0].Quote = "missing quotation that is not there"
	r.Facts = append(r.Facts, Fact{Text: "Officers seized more plants.", Source: 2, Quote: "officers seized plants at the farm"})
	r.Updates = []ReportUpdate{
		{Report: "kept-report", Facts: []Fact{{Text: "Officers seized plants.", Source: 2, Quote: "officers seized plants at the farm"}}},
		{Report: "dropped-report", Facts: []Fact{{Text: "Invented.", Source: 2, Quote: "a quotation nobody published"}}},
	}
	initial := safeurl.Evidence{URL: "https://news.example/a", FinalURL: "https://news.example/a", Content: "Police raided a farm."}
	fetch := func(context.Context, string) (safeurl.Evidence, error) {
		return safeurl.Evidence{URL: "https://news.example/b", FinalURL: "https://news.example/b", Content: "Yesterday officers seized plants at the farm."}, nil
	}
	got, _, _, err := r.Confirm(context.Background(), initial, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 1 || len(got.Updates) != 1 || got.Updates[0].Report != "kept-report" || got.Updates[0].Facts[0].Source != 1 {
		t.Fatal(got.Sources, got.Updates)
	}
}
