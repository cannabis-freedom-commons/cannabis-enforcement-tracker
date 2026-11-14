package worker

import (
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
)

// A data quotation the sources do not contain is answered with their closest passage; an
// unrelated one gets no hint.
func TestClosestHintQuotesTheSource(t *testing.T) {
	sources := []safeurl.Evidence{{Content: "<p>Police arrested five men and seized 145.7 kg of ganja near the village.</p>"}}
	r := research.Result{DataQuotes: []research.DataQuote{{Field: "figures.arrested", Source: 1, Quote: "police arrested 5 men and seized 145.7 kg ganja"}}}
	hint := closestHint(sources, dataQuote(r, "figures.arrested"))
	if !strings.Contains(hint, "Police arrested five men and seized 145.7 kg of ganja near the village.") {
		t.Fatal(hint)
	}
	if closestHint(sources, "The court sentenced him to ten years in prison on Friday") != "" {
		t.Fatal("an unrelated quotation must not get a hint")
	}
}
