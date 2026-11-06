package research

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyFileClonesOrCopies(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	if err := os.WriteFile(source, []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" {
		if err := cloneFile(source, filepath.Join(dir, "c.md")); err != nil {
			t.Log("no copy-on-write clone here:", err) // not APFS: copyFile falls back to a copy
		}
	}
	if err := copyFile(source, target, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(source); string(b) != "report" {
		t.Fatal("writing the copy changed the original")
	}
	if info, _ := os.Stat(target); info.Mode().Perm() != 0644 {
		t.Fatal("permissions", info.Mode())
	}
}
