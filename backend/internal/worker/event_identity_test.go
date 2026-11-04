package worker

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
)

func TestCoverageDetectsRemovalsAndCountryStateChanges(t *testing.T) {
	beforeReports := []report{{Slug: "utah-case", Country: "us/ut", Digest: "one"}, {Slug: "country-case", Country: "us", Digest: "two"}, {Slug: "foreign-case", Country: "gb", Digest: "three"}}
	before := countryCoverage(beforeReports, "us/ca")
	if len(before) != 2 {
		t.Fatal("state research did not see related country coverage", before)
	}
	for _, tc := range []struct {
		name    string
		reports []report
		updates map[string]bool
		want    bool
	}{
		{"unchanged", beforeReports, nil, false},
		{"removed", beforeReports[1:], nil, true},
		{"moved outside country", []report{{Slug: "utah-case", Country: "ca", Digest: "one"}, beforeReports[1]}, nil, true},
		{"changed state report", []report{{Slug: "utah-case", Country: "us/ut", Digest: "changed"}, beforeReports[1]}, nil, true},
		{"unrelated country", append(append([]report(nil), beforeReports...), report{Slug: "foreign-new", Country: "de", Digest: "new"}), nil, false},
		{"validated incidental update", beforeReports[1:], map[string]bool{"utah-case": true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := coverageDiffers(tc.reports, "us/ca", before, tc.updates); got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}

func TestPublicationFencesRetiredAliasesAndAliasChanges(t *testing.T) {
	for _, test := range []string{"changed aliases", "retired slug", "retired code"} {
		t.Run(test, func(t *testing.T) {
			repo, _ := commitFixture(t)
			os.MkdirAll(filepath.Join(repo, "reports"), 0755)
			path := filepath.Join(repo, "src/lib/report-aliases.js")
			os.MkdirAll(filepath.Dir(path), 0755)
			os.WriteFile(path, []byte(`export default {"slugs":{"retired-case":"live-case"},"codes":{"ABC123":"live-case"}};`), 0644)
			name, code := "new-case", "XYZ123"
			if test == "retired slug" {
				name = "retired-case"
			}
			if test == "retired code" {
				code = "ABC123"
			}
			draft := eventDraft(t, name, code, "it")
			if test != "changed aliases" {
				_, draft.AliasDigest, _ = research.ReadReportAliases(repo)
			}
			content, err := os.ReadFile(filepath.Join(draft.Dir, draft.Rel))
			if err != nil {
				t.Fatal(err)
			}
			var changed *coverageChanged
			if err := checkPublicationCoverage(repo, draft, content); !errors.As(err, &changed) {
				t.Fatal("retired identity could publish", err)
			}
		})
	}
}

func TestOldQueuedDraftNeedsEventComparisonBeforePublication(t *testing.T) {
	repo, _ := commitFixture(t)
	original := eventDraft(t, "existing-case", "ABC123", "it")
	originalContent, err := os.ReadFile(filepath.Join(original.Dir, original.Rel))
	if err != nil {
		t.Fatal(err)
	}
	originalContent = []byte(strings.Replace(string(originalContent), "Police arrest a man with 71 grams", "Pozzuoli village farm raid", 1))
	os.MkdirAll(filepath.Dir(filepath.Join(repo, original.Rel)), 0755)
	os.WriteFile(filepath.Join(repo, original.Rel), originalContent, 0644)
	draft := eventDraft(t, "another-outlet", "XYZ123", "it")
	content, err := os.ReadFile(filepath.Join(draft.Dir, draft.Rel))
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), "Police arrest a man with 71 grams", "Pozzuoli village farm raid", 1))
	reports, err := Coverage(repo)
	if err != nil {
		t.Fatal(err)
	}
	draft.Coverage = countryCoverage(reports, "it")
	var changed *coverageChanged
	if err := checkPublicationCoverage(repo, draft, content); !errors.As(err, &changed) {
		t.Fatal("old unexamined draft could publish", err)
	}
	draft.IdentityResult = &research.Result{Facts: []research.Fact{{Text: "Police arrested a man."}}, EventComparisons: []research.EventComparison{{Report: "existing-case", Relation: "distinct_event", ExistingText: "Police arrested a man.", FactText: "Police arrested a man.", Difference: "The opened account identifies a separate incident and a different suspect."}}}
	// Serialize as a durable queued job: no in-memory confirmed flag survives.
	b, _ := json.Marshal(draft)
	var recovered draftJob
	if err := json.Unmarshal(b, &recovered); err != nil {
		t.Fatal(err)
	}
	if err := checkPublicationCoverage(repo, recovered, content); err != nil {
		t.Fatal("supported distinct event could not publish after recovery", err)
	}
	content = []byte(strings.Replace(string(content), "Police arrested a man.", "Police arrested a different person.", 1))
	if err := checkPublicationCoverage(repo, recovered, content); !errors.As(err, &changed) {
		t.Fatal("comparison not bound to drafted fact", err)
	}
}
