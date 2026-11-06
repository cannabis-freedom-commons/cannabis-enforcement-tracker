package research

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Development edits to the renderer, schema, editorial rules and reports must be
// validated too. Overlay only site inputs, never service state or credentials.
func snapshotSite(ctx context.Context, repo, dir string) error {
	allowed := []string{"src", "public", "scripts", "docs", "reports", ".claude/skills", ".opencode/agents", "package.json", "package-lock.json", "astro.config.mjs", "tsconfig.json"}
	cmd := exec.CommandContext(ctx, "git", append([]string{"ls-files", "-z", "--cached", "--others", "--exclude-standard", "--"}, allowed...)...)
	cmd.Dir = repo
	files, err := cmd.Output()
	if err != nil {
		return err
	}
	for _, path := range allowed {
		if err = os.RemoveAll(filepath.Join(dir, path)); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, name := range bytes.Split(files, []byte{0}) {
		path := string(name)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		if filepath.IsAbs(path) || strings.HasPrefix(filepath.Clean(path), "..") {
			return errors.New("invalid site snapshot path")
		}
		source := filepath.Join(repo, path)
		info, e := os.Lstat(source)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return errors.New("site snapshot requires regular files")
		}
		// A symlink in any parent must not bring host files into the snapshot.
		for parent := filepath.Dir(path); parent != "."; parent = filepath.Dir(parent) {
			stat, e := os.Lstat(filepath.Join(repo, parent))
			if e != nil {
				return e
			}
			if stat.Mode()&os.ModeSymlink != 0 {
				return errors.New("site snapshot directory is a symlink")
			}
		}
		target := filepath.Join(dir, path)
		if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
			return e
		}
		if e = copyFile(source, target, info.Mode().Perm()); e != nil {
			return e
		}
	}
	return nil
}

// copyFile copies one regular file: a copy-on-write clone where the file system has them (APFS),
// which takes no space until either side changes, otherwise its bytes.
func copyFile(source, target string, perm os.FileMode) error {
	if cloneFile(source, target) == nil {
		return os.Chmod(target, perm)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, perm)
}
