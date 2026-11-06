package research

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeTestReport(t *testing.T, dir, rel, front, body string) {
	t.Helper()
	path := filepath.Join(dir, "reports", rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\n"+front+"---\n"+body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReportLookupFindsAndReadsReports(t *testing.T) {
	dir := t.TempDir()
	writeTestReport(t, dir, "2026/11/cuiaba-lixeira-39-portions-arrest.md",
		"title: Teen arrested with 39 cannabis portions in Cuiabá's Lixeira\ncountry: br\ndate: 2026-11-04\ntype: arrest\nplace: Lixeira, Cuiabá, Mato Grosso\nfigures:\n  arrested: 1\ntags: [cuiaba, rotam]\nsources:\n  - url: https://www.hnt.com.br/policia/lixeira\n",
		"\nRotam officers arrested an 18-year-old in Lixeira with 39 portions of marijuana. [@1]\n")
	writeTestReport(t, dir, "2026/10/varzea-grande-skunk-arrest.md",
		"title: Skunk seized in Várzea Grande\ncountry: br\ndate: 2026-10-30\ntype: seizure\nplace: Várzea Grande, Mato Grosso\nsources:\n  - url: https://example.com/vg\n",
		"\nPolice seized skunk in Várzea Grande. [@1]\n")
	writeTestReport(t, dir, "2025/01/lisbon-hashish.md",
		"title: Hashish seized in Lisbon\ncountry: pt\ndate: 2025-01-10\nplace: Lisbon\nsources:\n  - url: https://example.pt/a\n",
		"\nHashish was seized in Lisbon. [@1]\n")
	l := newReportLookup(dir)
	args, _ := json.Marshal(map[string]any{"country": "br", "date": "2026-11-05", "title": "Rotam arrests 18-year-old with 39 marijuana portions in Cuiabá's Lixeira", "place": "Lixeira, Cuiabá", "source_urls": []string{"https://www.hnt.com.br/policia/lixeira"}})
	out := l.find(args)
	required, other, _ := strings.Cut(out, "Other possible matches")
	if !strings.Contains(required, "cuiaba-lixeira-39-portions-arrest") || !strings.Contains(required, "shared source") || !strings.Contains(required, "tags: cuiaba rotam") {
		t.Fatal("the matcher's required comparison is missing", out)
	}
	if strings.Contains(out, "lisbon-hashish") {
		t.Fatal("another country was listed", out)
	}
	if strings.Contains(other, "varzea-grande") {
		t.Fatal("a report sharing no distinctive word was listed", out)
	}
	wide, _ := json.Marshal(map[string]any{"country": "br", "keywords": "Várzea Grande"})
	if out := l.find(wide); !strings.Contains(out, "varzea-grande-skunk-arrest") || strings.Contains(out, "within") {
		t.Fatal("keyword search without a date", out)
	}
	if out := l.get(json.RawMessage(`{"slug":"existing-reports/cuiaba-lixeira-39-portions-arrest.md"}`)); !strings.Contains(out, "39 portions of marijuana. [@1]") {
		t.Fatal("report not read", out)
	}
	if out := l.get(json.RawMessage(`{"slug":"../../etc/passwd"}`)); !strings.Contains(out, "No report has the slug") {
		t.Fatal("path outside the reports", out)
	}
}

func TestFindReportsWithTheDraftMatchesTheCheck(t *testing.T) {
	dir := t.TempDir()
	writeTestReport(t, dir, "2026/11/cuiaba-lixeira-39-portions-arrest.md",
		"title: Teen arrested with 39 cannabis portions in Cuiabá's Lixeira\ncountry: br\ndate: 2026-11-04\ntype: arrest\nplace: Lixeira, Cuiabá, Mato Grosso\nsources:\n  - url: https://www.hnt.com.br/policia/lixeira\n",
		"\nRotam officers arrested an 18-year-old in Lixeira with 39 portions of marijuana. [@1]\n")
	l := newReportLookup(dir)
	// the first guess shares nothing with the report; the draft cites its source
	if out := l.find(json.RawMessage(`{"country":"br","title":"Police operation in Mato Grosso"}`)); strings.Contains(out, "will require") {
		t.Fatal("the first guess should require nothing", out)
	}
	draft := `{"outcome":"new_event","country":"br","date":"2026-11-05","title":"Police operation in Mato Grosso","place":"Cuiabá","facts":[{"text":"Officers made an arrest."}],"sources":[{"url":"https://www.hnt.com.br/policia/lixeira"}]}`
	var r Result
	if err := json.Unmarshal([]byte(draft), &r); err != nil {
		t.Fatal(err)
	}
	reports, _ := LoadReports(dir)
	want := EventCandidates(r, reports)
	if len(want) != 1 {
		t.Fatalf("fixture: the check should require one comparison, got %v", want)
	}
	for _, arg := range []string{`{"result":` + draft + `}`, `{"result":` + strconv.Quote(draft) + `}`} {
		out := l.find(json.RawMessage(arg))
		required, _, _ := strings.Cut(out, "Other possible matches")
		if !strings.Contains(required, "will require") || !strings.Contains(required, want[0].Report) {
			t.Fatal("the draft's required comparison is missing", out)
		}
	}
	// the check names every missing comparison with its date and title
	err := CheckEventComparisons(r, reports, false)
	if err == nil || !strings.Contains(err.Error(), "2026-11-04") || !strings.Contains(err.Error(), "Teen arrested with 39 cannabis portions") {
		t.Fatalf("missing comparison message: %v", err)
	}
}
