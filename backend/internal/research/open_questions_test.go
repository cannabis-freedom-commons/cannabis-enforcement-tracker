package research

import (
	"errors"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"gopkg.in/yaml.v3"
)

// writeQuestionsReport puts a valid report with these open questions into a fresh checkout.
func writeQuestionsReport(t *testing.T, questions int) (string, Result) {
	t.Helper()
	dir := t.TempDir()
	r := valid()
	r.Facts[0].Text = strings.Repeat("Police raided the farm and arrested two men. ", 6)
	for i := 0; i < questions; i++ {
		r.Uncertainties = append(r.Uncertainties, fmt.Sprintf("Original unresolved question %d.", i))
	}
	path := filepath.Join(dir, "reports/2026/01", r.Slug+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(r.Markdown()), 0644); err != nil {
		t.Fatal(err)
	}
	return dir, r
}

func questionsOf(t *testing.T, content string) []string {
	t.Helper()
	var header struct {
		Questions []string `yaml:"open_questions"`
	}
	if err := yaml.Unmarshal([]byte(strings.SplitN(content, "---", 3)[1]), &header); err != nil {
		t.Fatal(err)
	}
	return header.Questions
}

// Updates no longer pile up open questions: adding past the bound asks for the revised list.
func TestUpdateQuestionsAreBounded(t *testing.T) {
	dir, r := writeQuestionsReport(t, 11)
	u := ReportUpdate{Report: r.Slug, Facts: r.Facts, Uncertainties: []string{"Original  unresolved question 0.", "A genuinely new discrepancy remains unresolved."}}
	d, err := UpdateReport(dir, u, r.Sources, nil)
	if err != nil {
		t.Fatal("one new question within the bound", err)
	}
	if q := questionsOf(t, d.Content); len(q) != 12 || q[11] != u.Uncertainties[1] {
		t.Fatal(q)
	}
	if err := os.WriteFile(filepath.Join(dir, d.Path), []byte(d.Content), 0644); err != nil {
		t.Fatal(err)
	}
	u.Uncertainties = []string{"Yet another account gives a different date."}
	if _, err := UpdateReport(dir, u, r.Sources, nil); !errors.Is(err, ErrOpenQuestions) {
		t.Fatal("a thirteenth question must ask for the revised list", err)
	}
	// the revised list replaces the old one, however long it was
	revised := []string{"Accounts disagree on the date.", "No one has been charged."}
	u.Uncertainties, u.OpenQuestions = nil, &revised
	d, err = UpdateReport(dir, u, r.Sources, nil)
	if err != nil {
		t.Fatal(err)
	}
	if q := questionsOf(t, d.Content); strings.Join(q, "|") != strings.Join(revised, "|") {
		t.Fatal(q)
	}
	if err := CheckReport(repo, d.Path, d.Content); err != nil {
		t.Fatal(err)
	}
	// an overgrown legacy report can still gain a citation without new questions
	dir, r = writeQuestionsReport(t, 40)
	if _, err := UpdateReport(dir, ReportUpdate{Report: r.Slug, Facts: r.Facts}, r.Sources, nil); err != nil && !errors.Is(err, ErrUnchanged) {
		t.Fatal(err)
	}
	// and a revised list of at most 12 cleans it up
	short := []string{"No one has been charged."}
	d, err = UpdateReport(dir, ReportUpdate{Report: r.Slug, Facts: r.Facts, OpenQuestions: &short}, r.Sources, nil)
	if err != nil {
		t.Fatal(err)
	}
	if q := questionsOf(t, d.Content); len(q) != 1 {
		t.Fatal(q)
	}
	// an empty revised list removes them all
	none := []string{}
	if err := os.WriteFile(filepath.Join(dir, d.Path), []byte(d.Content), 0644); err != nil {
		t.Fatal(err)
	}
	d, err = UpdateReport(dir, ReportUpdate{Report: r.Slug, Facts: r.Facts, OpenQuestions: &none}, r.Sources, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(d.Content, "open_questions") {
		t.Fatal("empty revised list left the key")
	}
	// one model result is bounded too
	r = valid()
	for i := 0; i < MaxOpenQuestions+1; i++ {
		r.Uncertainties = append(r.Uncertainties, fmt.Sprintf("Question %d.", i))
	}
	if r.Validate(map[string]bool{"th": true}) == nil {
		t.Fatal("unbounded questions in one result")
	}
}

// Notes about the research are not questions about the case.
func TestResearchNotesAreNotOpenQuestions(t *testing.T) {
	for _, q := range []string{
		"The submitted page says six hours, while the existing report says seven.",
		"The page displays a photo without a caption, so no media entry is listed for this source.",
	} {
		r := valid()
		r.Uncertainties = []string{q}
		if r.Validate(map[string]bool{"th": true}) == nil {
			t.Fatal("accepted", q)
		}
		u := ReportUpdate{Report: r.Slug, Facts: r.Facts, Uncertainties: []string{q}}
		if u.validate(len(r.Sources), nil) == nil {
			t.Fatal("update accepted", q)
		}
	}
	r := valid()
	r.Uncertainties = []string{"Accounts give the walk as six to nine hours."}
	if err := r.Validate(map[string]bool{"th": true}); err != nil {
		t.Fatal(err)
	}
}

// A new detail joins the paragraph of the sentence it follows instead of a new paragraph at the end.
func TestNewDetailFollowsItsSentence(t *testing.T) {
	body := "Police raided the farm. [@1] Two men were held.\n\nThe court will rule later. [@1]"
	got, ok := insertAfter(body, "Police raided the farm", "They found 40 plants. [@2]")
	if !ok || got != "Police raided the farm. [@1] They found 40 plants. [@2] Two men were held.\n\nThe court will rule later. [@1]" {
		t.Fatal(got)
	}
	if _, ok := insertAfter(body, "A sentence that is not there", "x"); ok {
		t.Fatal("matched a missing sentence")
	}
}

func TestOpenQuestionsDoNotInvalidateSourcedReportAndSurviveMarkdown(t *testing.T) {
	r := valid()
	r.Uncertainties = []string{"Outlets disagree on whether the truck carried cabbage or cauliflower.", "The court outcome is not yet known."}
	if err := r.Validate(map[string]bool{"th": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Verify(context.Background(), safeurl.Evidence{URL: r.Sources[0].URL, Content: r.Facts[0].Quote}); err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(r.Markdown(), "---", 3)
	var header struct {
		Questions []string `yaml:"open_questions"`
		Status    string   `yaml:"status"`
	}
	if err := yaml.Unmarshal([]byte(parts[1]), &header); err != nil {
		t.Fatal(err)
	}
	if len(header.Questions) != 2 || header.Questions[0] != r.Uncertainties[0] || header.Status != "reported" {
		t.Fatal(header)
	}
	r.Facts[0].Quote = "Unsupported central factual claim."
	if _, err := r.Verify(context.Background(), safeurl.Evidence{URL: r.Sources[0].URL, Content: "Nothing supporting that claim."}); err == nil {
		t.Fatal("open questions bypassed source verification")
	}
}
func TestExistingCaseRetainsAndAddsOpenQuestions(t *testing.T) {
	dir := t.TempDir()
	r := valid()
	r.Outcome = "existing_case"
	for i := range r.Facts {
		r.Facts[i].NewDetail = true
	}
	r.ExistingReport = r.Slug
	path := filepath.Join(dir, "reports/2026/01", r.Slug+".md")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("---\ndate: 2026-01-01\ncountry: th\ntype: raid\nstatus: reported\nsources:\n  - title: News\n    url: https://news.example/a\n    publisher: Publisher\nopen_questions:\n  - Previous question.\n---\nOriginal prose. [@1]\n"), 0644)
	r.Uncertainties = []string{"Previous question.", "New question."}
	d, err := PrepareDraft(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	var header struct {
		Questions []string `yaml:"open_questions"`
	}
	if err = yaml.Unmarshal([]byte(strings.SplitN(d.Content, "---", 3)[1]), &header); err != nil {
		t.Fatal(err)
	}
	if len(header.Questions) != 2 || !strings.Contains(d.Content, "Original prose.") {
		t.Fatal(d.Content)
	}
}
func TestSnapshotUsesCurrentSiteInputsWithoutCopyingCredentials(t *testing.T) {
	repo := t.TempDir()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
	git("init")
	git("config", "user.name", "Fixture")
	git("config", "user.email", "fixture@localhost")
	os.MkdirAll(filepath.Join(repo, "src"), 0755)
	os.WriteFile(filepath.Join(repo, "src/schema.txt"), []byte("old schema"), 0644)
	git("add", ".")
	git("commit", "-m", "Initial")
	os.WriteFile(filepath.Join(repo, "src/schema.txt"), []byte("new schema with questions"), 0644)
	os.WriteFile(filepath.Join(repo, "src/new-renderer.txt"), []byte("new component"), 0644)
	os.WriteFile(filepath.Join(repo, ".env"), []byte("secret"), 0600)
	if err := snapshotSite(context.Background(), repo, dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "src/schema.txt"))
	if string(data) != "new schema with questions" {
		t.Fatal("snapshot used stale committed code")
	}
	if _, err := os.Stat(filepath.Join(dir, "src/new-renderer.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); !os.IsNotExist(err) {
		t.Fatal("copied host credentials")
	}
}
