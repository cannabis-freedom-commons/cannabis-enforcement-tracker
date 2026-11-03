package research

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const repo = "../../.." // the repository root, from this package

// every report in the repository passes the site's own schema and tests, so it must pass here too:
// the check may not be stricter than the site
func TestCheckReportAcceptsEveryExistingReport(t *testing.T) {
	n := 0
	filepath.WalkDir(filepath.Join(repo, "reports"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		b, _ := os.ReadFile(path)
		rel, _ := filepath.Rel(repo, path)
		if err := CheckReport(repo, rel, string(b)); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
		n++
		return nil
	})
	if n < 100 {
		t.Fatal("reports not found", n)
	}
}

func TestCheckReportNamesWhatIsWrong(t *testing.T) {
	r := valid()
	r.Facts = []Fact{{Text: strings.Repeat("Police raided the farm and arrested two men. ", 6), Source: 1, Quote: "q", Paragraph: 1}}
	r.Submission = "0123456789abcdef0123456789abcdef"
	good := r.Markdown()
	if err := CheckReport(repo, "reports/2026/01/example-raid.md", good); err != nil {
		t.Fatal("a generated report was refused:", err)
	}
	for _, c := range []struct {
		change     func(string) string
		path, want string
	}{
		{func(s string) string { return strings.Replace(s, "code: MZ48CH\n", "", 1) }, "", "code"},
		{func(s string) string { return strings.Replace(s, "code: MZ48CH", "code: 123456", 1) }, "", "code"},
		{func(s string) string { return strings.Replace(s, "country: th", "country: atlantis", 1) }, "", "country"},
		{func(s string) string { return strings.Replace(s, "type: raid", "type: picnic", 1) }, "", "type"},
		{func(s string) string {
			return strings.Replace(s, "status: reported", "status: verified\nmood: happy", 1)
		}, "", "mood: unknown field"},
		{func(s string) string { return strings.Replace(s, " [@1]", " [@2]", 1) }, "", "[@2] names no source"},
		{func(s string) string {
			return strings.Replace(s, "status: reported\n", "status: reported\ntags: [Hong Kong]\n", 1)
		}, "", "tags[0]"},
		{func(s string) string { return s }, "reports/2025/03/example-raid.md", "reports/<year>/<month>"},
		{func(s string) string { return strings.Replace(s, "created_by_model: fixture/research-model\n", "", 1) }, "", "created_by_model"},
	} {
		path := c.path
		if path == "" {
			path = "reports/2026/01/example-raid.md"
		}
		if err := CheckReport(repo, path, c.change(good)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("want %q, got %v", c.want, err)
		}
	}
}
