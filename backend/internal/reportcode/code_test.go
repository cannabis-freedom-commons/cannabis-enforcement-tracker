package reportcode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashMatchesJavaScript(t *testing.T) {
	for seed, want := range map[string]string{"example-raid": "MZ48CH", "example-raid#1": "WJS3NQ", "test-0": "7XC52R", "chennai-airport-1478g-hydro": "211101"} {
		if got := Of(seed); got != want {
			t.Fatalf("%s: %s, want %s", seed, got, want)
		}
	}
	if got := Available("example-raid", map[string]string{"MZ48CH": "other.md"}); got != "WJS3NQ" {
		t.Fatal(got)
	}
}

func TestCodesAreRequiredUniqueStringsAndSurviveMoves(t *testing.T) {
	dir := t.TempDir()
	old := "reports/2026/01/old.md"
	os.MkdirAll(filepath.Dir(filepath.Join(dir, old)), 0755)
	content := "---\ncode: \"001234\"\n---\nReport.\n"
	os.WriteFile(filepath.Join(dir, old), []byte(content), 0644)
	if err := Check(dir, map[string]string{"reports/2026/02/renamed.md": content}, old); err != nil {
		t.Fatal(err)
	}
	for _, files := range []map[string]string{
		{"reports/2026/02/new.md": content},
		{"reports/2026/02/new.md": "---\ndate: 2026-02-01\n---\nReport.\n"},
		{"reports/2026/02/new.md": "---\ncode: 123456\n---\nReport.\n"},
		{"reports/2026/02/new.md": "---\ncode: INVALID\n---\nReport.\n"},
		{"reports/2026/02/new.md": "---\ncode: ABC123\n---\n", "reports/2026/02/other.md": "---\ncode: ABC123\n---\n"},
	} {
		if err := Check(dir, files); err == nil {
			t.Fatal("invalid code accepted", files)
		}
	}
}
