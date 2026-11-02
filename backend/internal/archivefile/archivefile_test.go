package archivefile

import (
	"bytes"
	"github.com/klauspost/compress/zstd"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestReadPlainOrZstdAndRootConfinement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.jsonl")
	input := bytes.Repeat([]byte("archived 大麻 evidence\n"), 100)
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	os.WriteFile(path+".zst", encoder.EncodeAll(input, nil), 0600)
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(f)
	f.Close()
	if err != nil || !bytes.Equal(got, input) {
		t.Fatal("compressed read", err)
	}
	os.WriteFile(path, []byte("live"), 0600)
	f, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err = io.ReadAll(f)
	f.Close()
	if err != nil || string(got) != "live" {
		t.Fatal("plain should take precedence", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	outside := filepath.Join(t.TempDir(), "private.zst")
	os.WriteFile(outside, encoder.EncodeAll(input, nil), 0600)
	os.Symlink(outside, filepath.Join(dir, "escape.zst"))
	if f, err := OpenRoot(root, "escape"); err == nil {
		f.Close()
		t.Fatal("compressed symlink escaped root")
	}
	os.WriteFile(filepath.Join(dir, "bad.zst"), []byte("not zstd"), 0600)
	f, err = Open(filepath.Join(dir, "bad"))
	if err == nil {
		_, err = io.ReadAll(f)
		f.Close()
	}
	if err == nil {
		t.Fatal("corrupt archive accepted")
	}
}
