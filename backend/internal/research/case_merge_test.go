package research

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExistingCaseMergesCorroborationWithoutRepeatingTheStory(t *testing.T) {
	dir := writeReport(t)
	path := filepath.Join(dir, "reports/2026/10/khust-five-years.md")
	original := strings.Replace(existingReport, "country: ua", "country: ua\ntype: sentence\ncreated_by_model: original/model#xhigh\n# Keep the original attribution and layout\nfigures:\n  arrested: 1", 1)
	os.WriteFile(path, []byte(original), 0644)
	r := valid()
	r.Outcome = "existing_case"
	r.ExistingReport = "khust-five-years"
	r.Slug = r.ExistingReport
	r.Country = "ua"
	r.Date = "2026-10-30"
	r.Type = "sentence"
	r.Sources = []Source{{URL: "https://second.example/story", Title: "Second story", Publisher: "Second"}, {URL: "https://third.example/story", Title: "Third story", Publisher: "Third"}}
	r.Facts = []Fact{
		{Text: "A man received a five-year prison sentence in Khust.", ExistingText: "The Khust District Court sentenced a man to five years in prison.", Source: 1, Quote: "A man received a five-year prison sentence in Khust."},
		{Text: "He remains in custody until the verdict takes effect.", NewDetail: true, Source: 1, Quote: "He remains in custody until the verdict takes effect."},
		{Text: "He remains in custody until the verdict takes effect.", NewDetail: true, Source: 2, Quote: "He remains in custody until the verdict takes effect."},
	}
	r.Figures = &Figures{Prison: "5 years"}
	d, err := PrepareDraft(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"coordinates: [48.1765, 23.2912]", "# Keep the original attribution and layout", "created_by_model: original/model#xhigh", "summary: >-", "prison: 5 years", "arrested: 1", "five years in prison. [@1,2]", "effect. [@2,3]"} {
		if !strings.Contains(d.Content, want) {
			t.Fatalf("lost %q in %s", want, d.Content)
		}
	}
	if strings.Contains(d.Content, "## Update") || strings.Contains(d.Content, "A man received") || strings.Contains(d.Content, "{\"sources\"") || strings.Count(d.Content, "He remains in custody") != 1 {
		t.Fatal("duplicated or reformatted report", d.Content)
	}
	os.WriteFile(path, []byte(d.Content), 0644)
	if _, err = PrepareDraft(dir, r); !errors.Is(err, ErrUnchanged) {
		t.Fatal("repeated merge was not idempotent", err)
	}
}

func TestMergeDoesNotAppendUnanchoredParaphrases(t *testing.T) {
	u := ReportUpdate{Report: "khust-five-years", Facts: []Fact{{Text: "A man from Khust received a sentence of five years.", Source: 1, Quote: "A man from Khust received a sentence of five years."}}}
	sources := []Source{{URL: "https://second.example/story", Title: "Second story", Publisher: "Second"}}
	if _, err := UpdateReport(writeReport(t), u, sources, nil); err == nil {
		t.Fatal("unmarked paraphrase appended")
	}
	u.Facts[0].ExistingText = "The Khust District Court sentenced a man\nto five years in prison."
	d, err := UpdateReport(writeReport(t), u, sources, nil)
	if err != nil || strings.Contains(d.Content, "A man from Khust") {
		t.Fatal("line wrapping prevented corroboration", err, d.Content)
	}
	u.Facts[0].ExistingText = "An anchor that does not exist in the original report."
	if _, err := UpdateReport(writeReport(t), u, sources, nil); err == nil {
		t.Fatal("missing anchor appended a paraphrase")
	}
}

func TestLegacyJSONHeaderIsConvertedBackToBlockYAML(t *testing.T) {
	dir := writeReport(t)
	path := filepath.Join(dir, "reports/2026/10/khust-five-years.md")
	os.WriteFile(path, []byte("---\n"+`{"date":"2026-10-30","country":"ua","type":"sentence","created_by_model":"original/model","title":"Khust sentence","status":"reported","sources":[{"url":"https://first.example/story","title":"First story","publisher":"First"}]}`+"\n---\n\nPolice arrested him in July. [@1]\n"), 0644)
	u := ReportUpdate{Report: "khust-five-years", Facts: []Fact{{Text: "Police arrested him in July.", Source: 1, Quote: "Police arrested him in July 2026."}}}
	d, err := UpdateReport(dir, u, []Source{{URL: "https://second.example/story", Title: "Second", Publisher: "Second"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var header map[string]any
	if err = yaml.Unmarshal([]byte(strings.SplitN(d.Content, "---", 3)[1]), &header); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(strings.TrimSpace(strings.SplitN(d.Content, "---", 3)[1]), "{") || header["created_by_model"] != "original/model" || !strings.Contains(d.Content, "sources:\n") {
		t.Fatal(d.Content)
	}
}

func TestResearchInputIncludesOriginalReportsForSentenceMatching(t *testing.T) {
	dir := writeReport(t)
	files, err := existingReportFiles(dir)
	if err != nil || string(files["existing-reports/khust-five-years.md"]) != existingReport {
		t.Fatal("original report missing from read-only evidence input", files, err)
	}
}

func TestMergeReportWithInlineFrontMatterHyphens(t *testing.T) {
	dir := writeReport(t)
	path := filepath.Join(dir, "reports/2026/10/khust-five-years.md")
	original := strings.Replace(existingReport, "First story", "Death penalty news----worldwide", 1)
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	u := ReportUpdate{Report: "khust-five-years", Facts: []Fact{{Text: "Police arrested him in July.", Source: 1, Quote: "Police arrested him in July 2026."}}}
	d, err := UpdateReport(dir, u, []Source{{URL: "https://second.example/story", Title: "Second", Publisher: "Second"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Death penalty news----worldwide", "Police arrested him in July. [@1,2]"} {
		if !strings.Contains(d.Content, want) {
			t.Fatalf("lost %q: %s", want, d.Content)
		}
	}
}
