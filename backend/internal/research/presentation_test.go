package research

import (
	"strings"
	"testing"
)

func TestCleanPresentationDoesNotRepairOrVerifyFacts(t *testing.T) {
	r := valid()
	r.Explanation = "The existing `example-raid` report has **another** opened account."
	r.DecisionTitle = "Another `account`"
	before := r.Facts[0]
	r.CleanPresentation()
	if err := r.Validate(map[string]bool{"th": true}); err != nil {
		t.Fatal(err)
	}
	if r.Facts[0] != before || r.Explanation != "The existing example-raid report has another opened account." {
		t.Fatal("changed evidence or lost explanation", r)
	}
	for _, outcome := range []string{"new_event", "additional_source", "existing_case", "not_relevant", "duplicate", "insufficient_evidence"} {
		r.Outcome, r.Explanation = outcome, ""
		r.CleanPresentation()
		if (r.Explanation != "") != r.Drafts() {
			t.Fatal("invented a rejection rationale or omitted the draft operation", outcome, r.Explanation)
		}
	}
	r = valid()
	r.Explanation = strings.Repeat("大麻", 700)
	if err := r.Validate(map[string]bool{"th": true}); err != nil {
		t.Fatal("character limit was treated as a byte limit", err)
	}
	r.Explanation += strings.Repeat("x", 101)
	if r.Validate(map[string]bool{"th": true}) == nil {
		t.Fatal("oversized explanation accepted")
	}
}
