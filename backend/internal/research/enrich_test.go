package research

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
)

func TestEnrichAddsOnlyQuotedValuesAndReportsConflicts(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS local runner")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"src/data/countries.json", "src/data/vocabularies.json", "src/data/agencies.json", ".opencode/agents/enforcement-researcher.md"} {
		b, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	report := "---\ncode: 7K3M9Q\ndate: 2026-01-02\ncountry: th\ntype: raid\ntitle: Police raid a cannabis farm\nsummary: Police raided a farm.\nfigures:\n  arrested: 3\nstatus: reported\ncreated_by_model: fixture/model\nsources:\n  - title: News story\n    url: https://news.example/a\n    publisher: Publisher\n---\n\nPolice raided the farm in the province after weeks of surveillance, the provincial police said in a statement on the day of the raid, adding that the harvest was destined for buyers in the city, where demand had grown, according to the statement. [@1]\n"
	rel := "reports/2026/01/example-raid.md"
	if err = os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, rel), []byte(report), 0644); err != nil {
		t.Fatal(err)
	}
	answer := `{"figures":{"arrested":2,"kg_seized":40},"details":{"agencies":["th-oncb"],"setting":["farm"],"case_stage":"charged"},"data_quotes":[{"field":"figures.arrested","source":1,"quote":"Two men were arrested"},{"field":"figures.kg_seized","source":1,"quote":"40 kilograms of marijuana"},{"field":"details.agencies","source":1,"quote":"the ONCB said"},{"field":"details.setting","source":1,"quote":"raided the farm"},{"field":"details.case_stage","source":1,"quote":"they were charged in court"}]}`
	event := `{"type":"text","part":{"text":` + quoteJSON(answer) + `}}`
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 1.0.0; exit 0; fi\ncat <<'RESPONSE'\n" + event + "\nRESPONSE\n"
	if err = os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(bin, "provider.json")
	if err = os.WriteFile(empty, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	r := Runner{Config: config.Config{ResearchRunner: "local", OpenCodeBinary: filepath.Join(bin, "opencode"), OpenCodeConfigFile: empty, OpenCodeAuthFile: filepath.Join(bin, "none"), ResearchModel: "fixture/model", Workspaces: filepath.Join(dir, "jobs")}}
	page := safeurl.Evidence{URL: "https://news.example/a", Content: "Police raided the farm in the province. Two men were arrested and 40 kilograms of marijuana were seized, the ONCB said."}
	got, err := r.Enrich(context.Background(), dir, rel, map[int]safeurl.Evidence{1: page})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Added, ",") != "details.agencies,details.setting,figures.kg_seized" {
		t.Fatal("added", got.Added)
	}
	if len(got.Conflicts) != 1 || !strings.Contains(got.Conflicts[0], "figures.arrested: report 3, source 2") {
		t.Fatal("conflicts", got.Conflicts)
	}
	if strings.Join(got.LeftOut, ",") != "details.case_stage" {
		t.Fatal("left out", got.LeftOut)
	}
	for _, want := range []string{"arrested: 3", "kg_seized: 40", "agencies: [th-oncb]", "details.agencies: [1]", "figures.kg_seized: [1]"} {
		if !strings.Contains(got.Content, want) {
			t.Fatal("missing", want, "\n", got.Content)
		}
	}
	if strings.Contains(got.Content, "figures.arrested") || strings.Contains(got.Content, "case_stage") {
		t.Fatal("kept an unsupported or conflicting value\n", got.Content)
	}
}

func quoteJSON(s string) string {
	b := strings.Builder{}
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
