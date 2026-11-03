package research

import (
	"context"
	"encoding/json"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func valid() Result {
	return Result{CreatedByModel: "fixture/research-model", Outcome: "new_event", Country: "th", Date: "2026-01-02", Type: "raid", Slug: "example-raid", Title: "Police raid a cannabis farm", Summary: "Police raided a farm.", Explanation: "A sourced enforcement incident.", Place: "Chiang Mai", Coordinates: []float64{18.79, 98.98}, Precision: "locality", Sources: []Source{{URL: "https://news.example/a", Title: "News story", Publisher: "Publisher"}}, Facts: []Fact{{Text: "Police raided the farm.", Source: 1, Quote: "Police raided the farm in the province."}}}
}
func TestStrictAndValidation(t *testing.T) {
	r := valid()
	b, _ := json.Marshal(r)
	parsed, err := Strict(b)
	parsed.CreatedByModel = r.CreatedByModel // authoritative service attribution is applied after parsing
	if err != nil || parsed.Validate(map[string]bool{"th": true}) != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Result){func(r *Result) { r.CreatedByModel = "" }, func(r *Result) { r.Country = "invented" }, func(r *Result) { r.Date = "1999-01-01" }, func(r *Result) { r.Slug = "../../secret" }, func(r *Result) { r.Facts[0].Source = 2 }, func(r *Result) { r.Type = "invented" }, func(r *Result) { r.Facts[0].Text = "<script>steal()</script>" }} {
		v := valid()
		mutate(&v)
		if v.Validate(map[string]bool{"th": true}) == nil {
			t.Fatal(v)
		}
	}
	if _, err = Strict([]byte(`{"outcome":"needs_review","command":"push main"}`)); err == nil {
		t.Fatal("unknown fields accepted")
	}
}
func TestQuoteMustBeInFetchedEvidence(t *testing.T) {
	r := valid()
	initial := safeurl.Evidence{URL: r.Sources[0].URL, Content: "Unrelated article"}
	if _, err := r.Verify(context.Background(), initial); err == nil {
		t.Fatal("hallucinated quote accepted")
	}
	initial.Content = r.Facts[0].Quote
	if _, err := r.Verify(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
}
func TestExecutionStreamIsNotResult(t *testing.T) {
	r := valid()
	b, _ := json.Marshal(r)
	event, _ := json.Marshal(map[string]any{"type": "text", "part": map[string]string{"text": string(b)}})
	final, err := FinalText(append(event, '\n'))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Strict(final); err != nil {
		t.Fatal(err)
	}
	if _, err = FinalText([]byte(`{"type":"step_finish"}`)); err == nil {
		t.Fatal("partial output accepted")
	}
	if _, err = FinalText([]byte(`{"type":"error"}`)); err == nil {
		t.Fatal("error ignored")
	}
}
func TestExistingCaseDraftPreservesCitationsAndSlug(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "reports/2026/01/example-raid.md")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0755); err != nil {
		t.Fatal(err)
	}
	original := `---
code: 7KQ2MX
date: 2026-01-02
country: th
type: arrest
title: Earlier arrest
summary: Earlier reporting.
status: verified
sources:
  - url: https://news.example/earlier
    title: Earlier story
    publisher: Publisher
---

Police arrested a person. [@1]
`
	if err := os.WriteFile(oldPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	r := valid()
	r.Outcome = "existing_case"
	for i := range r.Facts {
		r.Facts[i].NewDetail = true
	}
	r.ExistingReport = r.Slug
	r.Date = "2026-02-03"
	r.Type = "sentence"
	if err := r.Validate(map[string]bool{"th": true}); err != nil {
		t.Fatal(err)
	}
	d, err := PrepareDraft(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	if d.Path != "reports/2026/02/example-raid.md" || d.PreviousPath != "reports/2026/01/example-raid.md" || !strings.Contains(d.Content, "Police arrested a person. [@1]") || !strings.Contains(d.Content, r.Facts[0].Text+" [@2]") {
		t.Fatal(d)
	}
	parts := strings.SplitN(d.Content, "---", 3)
	var header map[string]any
	if err = yaml.Unmarshal([]byte(parts[1]), &header); err != nil {
		t.Fatal(err)
	}
	if header["code"] != "7KQ2MX" || header["status"] != "reported" || header["date"].(time.Time).Format("2006-01-02") != "2026-02-03" || header["type"] != "sentence" {
		t.Fatal(header)
	}
	r.Country = "us"
	if _, err = PrepareDraft(dir, r); err == nil {
		t.Fatal("cross-country match accepted")
	}
	r.Country = "th"
	r.Date = "2025-12-01"
	if _, err = PrepareDraft(dir, r); err == nil {
		t.Fatal("earlier step replaced latest date")
	}
}

func TestCleanTagsMendsInsteadOfRefusing(t *testing.T) {
	got := CleanTags([]string{"Hong Kong", "香港", "São Paulo", "drug_trafficking", "hong-kong", "--x--", "Customs/Excise"})
	want := []string{"hong-kong", "sao-paulo", "drug-trafficking", "x", "customs-excise"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatal(got)
	}
	r := valid()
	r.Tags = CleanTags([]string{"Hong Kong", "大麻"})
	if r.Validate(map[string]bool{"th": true}) != nil {
		t.Fatal("cleaned tags still refused")
	}
}

func TestCapitalPunishmentReports(t *testing.T) {
	for _, typ := range []string{"sentence", "execution"} {
		t.Run(typ, func(t *testing.T) {
			r := valid()
			r.Type = typ
			r.Also = []string{"execution"}
			if typ == "execution" {
				r.Also = []string{"sentence"}
			}
			if err := r.Validate(map[string]bool{"th": true}); err != nil {
				t.Fatal(err)
			}
			draft := r.Markdown()
			if !strings.Contains(draft, "type: "+typ) {
				t.Fatal(draft)
			}
		})
	}
}
