// Package reportcode assigns the permanent CET identifiers stored in report front matter.
package reportcode

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/frontmatter"
	"gopkg.in/yaml.v3"
)

const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var pattern = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{6}$`)

// Of matches src/lib/code.js: the first 30 SHA-256 bits in Crockford base32.
func Of(seed string) string {
	hash := sha256.Sum256([]byte(seed))
	n := binary.BigEndian.Uint32(hash[:4]) >> 2
	var code [6]byte
	for i := len(code) - 1; i >= 0; i-- {
		code[i] = alphabet[n&31]
		n >>= 5
	}
	return string(code[:])
}

func Available(seed string, used map[string]string) string {
	for i := 0; ; i++ {
		candidate := seed
		if i > 0 {
			candidate = fmt.Sprintf("%s#%d", seed, i)
		}
		if code := Of(candidate); used[code] == "" {
			return code
		}
	}
}

// Read returns empty for a legacy report without a code. Present values must be strings.
func Read(content string) (string, error) {
	parts := frontmatter.Split(content)
	if len(parts) != 3 {
		return "", errors.New("invalid report front matter")
	}
	var header map[string]any
	if err := yaml.Unmarshal([]byte(parts[1]), &header); err != nil {
		return "", err
	}
	value, exists := header["code"]
	if !exists {
		return "", nil
	}
	code, ok := value.(string)
	if !ok || !pattern.MatchString(code) {
		return "", errors.New("code: required string of six Crockford base32 characters")
	}
	return code, nil
}

// Index includes working-tree reports, reserving codes even for uncommitted reports.
// Excluded paths are repository-relative files being replaced or moved by one publication.
func Index(dir string, exclude ...string) (map[string]string, error) {
	skipped := map[string]bool{}
	for _, path := range exclude {
		skipped[filepath.ToSlash(path)] = true
	}
	used := map[string]string{}
	err := filepath.WalkDir(filepath.Join(dir, "reports"), func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) && path == filepath.Join(dir, "reports") {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() || !strings.HasSuffix(path, ".md") || skipped[rel] {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		code, err := Read(string(b))
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		if code != "" {
			if other := used[code]; other != "" {
				return fmt.Errorf("code %s is shared by %s and %s", code, other, rel)
			}
			used[code] = rel
		}
		return nil
	})
	return used, err
}

// Check reserves every proposed file against the current repository and the other proposed files.
func Check(dir string, files map[string]string, removed ...string) error {
	for path := range files {
		removed = append(removed, path)
	}
	used, err := Index(dir, removed...)
	if err != nil {
		return err
	}
	for path, content := range files {
		code, err := Read(content)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if code == "" {
			return fmt.Errorf("%s: code is required", path)
		}
		if other := used[code]; other != "" {
			return fmt.Errorf("code %s is shared by %s and %s", code, other, path)
		}
		used[code] = path
	}
	return nil
}
