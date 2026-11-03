package research

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/reportcode"
)

func TestDraftAssignsCodeAndResolvesCollision(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "reports/2026/01"), 0755)
	os.WriteFile(filepath.Join(dir, "reports/2026/01/other.md"), []byte("---\ncode: MZ48CH\n---\nOther report.\n"), 0644)
	d, err := PrepareDraft(dir, valid())
	if err != nil {
		t.Fatal(err)
	}
	if code, err := reportcode.Read(d.Content); err != nil || code != "WJS3NQ" {
		t.Fatal(code, err)
	}
	r := valid()
	r.Slug = "chennai-airport-1478g-hydro"
	d, err = PrepareDraft(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	if code, err := reportcode.Read(d.Content); err != nil || code != "211101" {
		t.Fatal(code, err)
	}
	b, _ := json.Marshal(r)
	b = append([]byte(`{"code":"ABC123",`), b[1:]...)
	if _, err = Strict(b); err == nil {
		t.Fatal("model-assigned code accepted")
	}
}

func TestLegacyUpdateGainsCodeAndSubsequentUpdatesKeepIt(t *testing.T) {
	dir := writeReport(t)
	rel := "reports/2026/10/khust-five-years.md"
	os.WriteFile(filepath.Join(dir, rel), []byte(strings.Replace(existingReport, "code: 7KQ2MX\n", "", 1)), 0644)
	u := ReportUpdate{Report: "khust-five-years", Facts: []Fact{{Text: "The Khust District Court sentenced a man to five years in prison.", Source: 1, Quote: "sentenced to five years in prison"}}}
	sources := []Source{{URL: "https://second.example/story", Title: "Second account", Publisher: "Second"}}
	d, err := UpdateReport(dir, u, sources, nil)
	if err != nil {
		t.Fatal(err)
	}
	code, err := reportcode.Read(d.Content)
	if err != nil || code != reportcode.Of(u.Report) {
		t.Fatal(code, err)
	}
	os.WriteFile(filepath.Join(dir, rel), []byte(d.Content), 0644)
	u.Title = "Corrected court headline"
	d, err = UpdateReport(dir, u, sources, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reportcode.Read(d.Content); err != nil || got != code {
		t.Fatal(got, err)
	}
}
