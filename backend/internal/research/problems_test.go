package research

import (
	"encoding/json"
	"strings"
	"testing"
)

// the agent can only fix what the check names: each problem says which fact or source, and why
func TestResultProblemsNameWhatToFix(t *testing.T) {
	base := func() Result {
		return Result{Outcome: "new_event", Explanation: "A new seizure in Mersin.", CreatedByModel: "m", Date: "2026-11-06", Country: "tr", Slug: "mersin-48kg", Type: "seizure",
			Coordinates: []float64{36.8, 34.64}, Precision: "locality", Title: "Mersin police seize 48 kg of cannabis", Summary: "Police in Mersin seized 48 kg of cannabis.",
			Sources: []Source{{URL: "https://example.org/a", Title: "A", Publisher: "P"}, {URL: "https://example.org/b", Title: "B", Publisher: "P"}},
			Facts:   []Fact{{Source: 1, Text: "Police in Mersin seized 48 kg of cannabis.", Quote: "48 kilogram esrar ele geçirildi."}, {Source: 2, Text: "Nineteen people were remanded in custody.", Quote: "19 kişi tutuklandı, dedi yetkililer."}}}
	}
	countries := map[string]bool{"tr": true}
	if err := base().Validate(countries); err != nil {
		t.Fatal("the base result must pass:", err)
	}
	for _, c := range []struct {
		edit func(*Result)
		want []string
	}{
		{func(r *Result) { r.Facts[1].Source = 1 }, []string{"source 2 (https://example.org/b) supports no fact", "remove it from sources"}},
		{func(r *Result) { r.Facts[1].Source = 3 }, []string{`fact 2 ("Nineteen people`, "source is 3", "lists 2 source"}},
		{func(r *Result) { r.Facts[0].Quote = "48 kg" }, []string{"fact 1", "quote must be 20–4000 characters", "it has 5"}},
		{func(r *Result) { r.Facts[0].Text = "Police seized [48 kg]." }, []string{"fact 1", `remove "["`}},
		{func(r *Result) { r.Facts[0].ExistingText = "x"; r.Facts[0].NewDetail = true }, []string{"fact 1", "both existing_text and new_detail"}},
		{func(r *Result) { r.Title = strings.Repeat("ş", 121) }, []string{"the title must be at most 120 characters (it has 121)"}},
		{func(r *Result) { r.Also = []string{"seizure"} }, []string{`also lists "seizure", which is already the type`}},
		{func(r *Result) {
			r.Updates = []ReportUpdate{{Report: "other", Facts: []Fact{{Source: 1, Text: "Police in Mersin seized 48 kg.", Quote: "short"}}}}
		}, []string{"update of other, fact 1", "quote must be"}},
	} {
		r := base()
		c.edit(&r)
		err := r.Validate(countries)
		if err == nil {
			t.Fatal("expected a problem for", c.want)
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%q does not say %q", err, w)
			}
		}
	}
	// 120 non-ASCII characters are within the limit, as the site counts them
	r := base()
	r.Title = strings.Repeat("ş", 120)
	if err := r.Validate(countries); err != nil {
		t.Fatal(err)
	}
}

func TestDetailsAcceptOneTermForAList(t *testing.T) {
	var d Details
	if err := json.Unmarshal([]byte(`{"force":"lethal","setting":["field"],"agencies":""}`), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Force) != 1 || d.Force[0] != "lethal" || len(d.Setting) != 1 || d.Agencies != nil {
		t.Fatalf("%+v", d)
	}
	if err := json.Unmarshal([]byte(`{"force":3}`), &d); err == nil {
		t.Fatal("a number is not a term")
	}
	// a report file still has to hold the list, as the site's schema requires
	if err := checkDetails(repo, map[string]any{"force": "lethal"}, map[string]bool{}); err == nil || !strings.Contains(err.Error(), "force must be a list") {
		t.Fatal(err)
	}
}
