package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageWithHyphensInSourceTitle(t *testing.T) {
	dir := t.TempDir()
	reports := filepath.Join(dir, "reports", "2005", "01")
	if err := os.MkdirAll(reports, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(reports, "example.md")
	content := "---\ndate: 2005-01-01\ncountry: sa\ntitle: Example\nsources:\n  - title: \"[Deathpenalty] death penalty news----worldwide\"\n    url: https://example.org/article\n---\n\nArticle. [@1]\n\n---\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := Coverage(dir)
	if err != nil || len(got) != 1 {
		t.Fatalf("coverage blocked: %v %v", got, err)
	}
	if got[0].Slug != "example" || len(got[0].Sources) != 1 || got[0].Sources[0].URL != "https://example.org/article" {
		t.Fatal(got)
	}
	if err := os.WriteFile(path, []byte("---\ntitle: \"unterminated\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Coverage(dir); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("missing offending report path: %v", err)
	}
}

func TestCoverageReadsRepositoryReports(t *testing.T) {
	reports, err := Coverage("../../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) == 0 {
		t.Fatal("no repository reports read")
	}
}
