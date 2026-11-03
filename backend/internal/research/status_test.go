package research

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"gopkg.in/yaml.v3"
)

func markdownStatus(t *testing.T, content string) string {
	t.Helper()
	var header struct {
		Status string `yaml:"status"`
	}
	if err := yaml.Unmarshal([]byte(strings.SplitN(content, "---", 3)[1]), &header); err != nil {
		t.Fatal(err)
	}
	return header.Status
}

func unavailableSource(context.Context, string) (safeurl.Evidence, error) {
	return safeurl.Evidence{}, errors.New("source unavailable")
}

func TestStatusFollowsRetainedQuotationChecks(t *testing.T) {
	for _, mode := range []string{"captured", "dropped fact", "blocked source", "cached source"} {
		t.Run(mode, func(t *testing.T) {
			r := valid()
			r.Uncertainties = []string{"No court outcome was reported; only one independent source was found."}
			initial := safeurl.Evidence{URL: r.Sources[0].URL, Content: r.Facts[0].Quote}
			var cached []safeurl.Evidence
			want := "verified"
			switch mode {
			case "dropped fact":
				r.Facts = append(r.Facts, Fact{Text: "Officers made a second arrest.", Source: 1, Quote: "a quotation that does not appear in the capture"})
			case "blocked source", "cached source":
				r.Sources = append(r.Sources, Source{URL: "https://second.example/story", Title: "Second story", Publisher: "Second"})
				r.Facts = append(r.Facts, Fact{Text: "Two men were arrested.", Source: 2, Quote: "Police arrested two men at the farm."})
				if mode == "blocked source" {
					want = "reported"
				} else {
					cached = []safeurl.Evidence{{URL: r.Sources[1].URL, Content: r.Facts[1].Quote}}
				}
			}
			got, _, check, err := r.Confirm(context.Background(), initial, unavailableSource, cached...)
			if err != nil || !check.Passed {
				t.Fatal(check, err)
			}
			if status := markdownStatus(t, got.Markdown()); status != want {
				t.Fatalf("got %s, want %s", status, want)
			}
			if status := markdownStatus(t, r.Markdown()); status != "reported" {
				t.Fatal("unconfirmed input was mutated", status)
			}
		})
	}
}

func TestVerificationCannotComeFromJSONOrSurviveWithoutRechecking(t *testing.T) {
	r := valid()
	initial := safeurl.Evidence{URL: r.Sources[0].URL, Content: r.Facts[0].Quote}
	checked, _, _, err := r.Confirm(context.Background(), initial, unavailableSource)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"status":"verified"`, `"verified":true`, `"confirmed":true`} {
		b, _ := json.Marshal(r)
		b = append([]byte("{"+field+","), b[1:]...)
		if _, err := Strict(b); err == nil {
			t.Fatal("model-set verification accepted", field)
		}
	}
	nested, _ := json.Marshal(r)
	nested = []byte(strings.Replace(string(nested), `"facts":[{`, `"facts":[{"confirmed":true,`, 1))
	if _, err := Strict(nested); err == nil {
		t.Fatal("model-set per-fact confirmation accepted")
	}
	b, _ := json.Marshal(checked)
	// Checkpoint/public JSON must not retain trusted per-fact verification.
	if strings.Contains(string(b), "confirmed") {
		t.Fatal("verification flag was serialized")
	}
	restored, err := Strict(b)
	if err != nil {
		t.Fatal(err)
	}
	if markdownStatus(t, restored.Markdown()) != "reported" {
		t.Fatal("JSON restored a verification flag")
	}
	again, _, _, err := restored.Confirm(context.Background(), initial, unavailableSource)
	if err != nil || markdownStatus(t, again.Markdown()) != "verified" {
		t.Fatal("captured evidence did not restore verification", err)
	}
	failed, _, _, err := checked.Confirm(context.Background(), safeurl.Evidence{}, unavailableSource)
	if err == nil || markdownStatus(t, failed.Markdown()) != "reported" {
		t.Fatal("stale verification survived failed recheck", err)
	}
}

func TestSourceUpdatesPreserveOnlySupportedVerification(t *testing.T) {
	for _, previous := range []string{"verified", "reported", "disputed"} {
		for _, outcome := range []string{"additional_source", "existing_case"} {
			for _, checked := range []bool{false, true} {
				t.Run(previous+"/"+outcome+"/"+map[bool]string{false: "unchecked", true: "checked"}[checked], func(t *testing.T) {
					dir := writeReport(t)
					path := filepath.Join(dir, "reports/2026/10/khust-five-years.md")
					original := strings.Replace(existingReport, "status: verified", "status: "+previous, 1)
					original = strings.Replace(original, "country: ua", "country: ua\ntype: sentence\ncreated_by_model: original/model", 1)
					os.WriteFile(path, []byte(original), 0644)
					r := valid()
					r.Outcome = outcome
					r.ExistingReport = "khust-five-years"
					r.Slug = r.ExistingReport
					r.Country = "ua"
					r.Type = "sentence"
					r.Date = "2026-10-30"
					r.Sources = []Source{{URL: "https://second.example/story", Title: "Second", Publisher: "Second"}}
					r.Facts = []Fact{{Text: "The Khust District Court sentenced a man to five years in prison.", Source: 1, Quote: "The Khust District Court sentenced a man to five years in prison."},
						{Text: "He remains in custody.", NewDetail: true, Source: 1, Quote: "He remains in custody until the verdict takes effect."}}
					if checked {
						var err error
						r, _, _, err = r.Confirm(context.Background(), safeurl.Evidence{URL: r.Sources[0].URL, Content: r.Facts[0].Quote + " " + r.Facts[1].Quote}, unavailableSource)
						if err != nil {
							t.Fatal(err)
						}
					}
					d, err := PrepareDraft(dir, r)
					if err != nil {
						t.Fatal(err)
					}
					want := "reported"
					if previous == "disputed" {
						want = "disputed"
					} else if previous == "verified" && checked {
						want = "verified"
					}
					if status := markdownStatus(t, d.Content); status != want {
						t.Fatalf("got %s, want %s", status, want)
					}
				})
			}
		}
	}
}

func TestIncidentalUpdateUsesItsOwnChecks(t *testing.T) {
	dir := writeReport(t)
	r := valid()
	r.Sources = append(r.Sources, Source{URL: "https://blocked.example/story", Title: "Second", Publisher: "Second"})
	r.Facts = append(r.Facts, Fact{Text: "Police caught a second man.", Source: 2, Quote: "Police caught a second man outside the farm."})
	r.Updates = []ReportUpdate{{Report: "khust-five-years", Facts: []Fact{{Text: "Police arrested him in July.", Source: 1, Quote: r.Facts[0].Quote}}}}
	got, _, _, err := r.Confirm(context.Background(), safeurl.Evidence{URL: r.Sources[0].URL, Content: r.Facts[0].Quote}, unavailableSource)
	if err != nil {
		t.Fatal(err)
	}
	d, err := PrepareDraft(dir, got)
	if err != nil {
		t.Fatal(err)
	}
	if markdownStatus(t, d.Content) != "reported" || len(d.Updates) != 1 || markdownStatus(t, d.Updates[0].Content) != "verified" {
		t.Fatal("main and incidental verification were mixed")
	}
}
