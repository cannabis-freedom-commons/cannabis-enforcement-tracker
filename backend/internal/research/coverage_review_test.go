package research

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageReviewPreservesAcceptedDraftMetadataAndResolvesClaimedCode(t *testing.T) {
	for _, collision := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserve", true: "code claimed by another report"}[collision], func(t *testing.T) {
			dir := t.TempDir()
			os.MkdirAll(filepath.Join(dir, "reports/2026/01"), 0755)
			r := valid()
			r.Code = "ABC123"
			r.AddedAt = "2026-01-02T12:34:56Z"
			if collision {
				os.WriteFile(filepath.Join(dir, "reports/2026/01/other.md"), []byte("---\ncode: ABC123\n---\nExisting report.\n"), 0644)
			}
			draft, err := PrepareDraft(dir, r)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(draft.Content, `added_at: "2026-01-02T12:34:56Z"`) {
				t.Fatal("coverage review reset the original addition time", draft.Content)
			}
			if kept := strings.Contains(draft.Content, "code: ABC123\n"); kept == collision {
				t.Fatal("wrong code preservation/collision handling", draft.Content)
			}
		})
	}
}
