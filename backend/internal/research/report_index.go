package research

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/frontmatter"
	"gopkg.in/yaml.v3"
)

// LoadReports indexes every report under repo/reports: its front matter, prose, the names and case
// labels in it, and a digest of the file.
func LoadReports(repo string) ([]ReportIndex, error) {
	out := []ReportIndex{}
	err := filepath.WalkDir(filepath.Join(repo, "reports"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parts := frontmatter.Split(string(b))
		if len(parts) != 3 {
			return fmt.Errorf("%s: invalid report front matter", path)
		}
		var r ReportIndex
		if err = yaml.Unmarshal([]byte(parts[1]), &r); err != nil {
			return fmt.Errorf("%s: invalid report front matter: %w", path, err)
		}
		r.Slug = strings.TrimSuffix(d.Name(), ".md")
		r.Body = parts[2]
		r.Names = IndexNames(r.Title + " " + r.Summary + " " + r.Body)
		r.CaseIDs = IndexCaseIDs(r.Title + " " + r.Summary + " " + r.Body)
		sum := sha256.Sum256(b)
		r.Digest = hex.EncodeToString(sum[:])
		out = append(out, r)
		return nil
	})
	return out, err
}
