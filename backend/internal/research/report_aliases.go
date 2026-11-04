package research

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ReportAliases struct {
	Slugs map[string]string `json:"slugs"`
	Codes map[string]string `json:"codes"`
}

// The site alias module is a JSON object with an export wrapper. Never execute
// JavaScript from a checkout to read it; fail closed on a malformed inventory.
func ReadReportAliases(repo string) (ReportAliases, string, error) {
	var aliases ReportAliases
	b, err := os.ReadFile(filepath.Join(repo, "src/lib/report-aliases.js"))
	if errors.Is(err, os.ErrNotExist) {
		return aliases, "", nil
	}
	if err != nil {
		return aliases, "", err
	}
	_, object, found := strings.Cut(string(b), "export default ")
	if !found {
		return aliases, "", errors.New("invalid report alias module")
	}
	object = strings.TrimSuffix(strings.TrimSpace(object), ";")
	if err = json.Unmarshal([]byte(object), &aliases); err != nil {
		return aliases, "", fmt.Errorf("invalid report alias inventory: %w", err)
	}
	sum := sha256.Sum256(b)
	return aliases, hex.EncodeToString(sum[:]), nil
}

func CheckRetiredIdentity(repo, slug, code string) error {
	aliases, _, err := ReadReportAliases(repo)
	if err != nil {
		return err
	}
	if target := aliases.Slugs[slug]; target != "" {
		return fmt.Errorf("retired report slug %s already redirects to %s; read the canonical report", slug, target)
	}
	if target := aliases.Codes[strings.ToUpper(code)]; code != "" && target != "" {
		return fmt.Errorf("retired report code already redirects to %s; preserve its canonical identity", target)
	}
	return nil
}
