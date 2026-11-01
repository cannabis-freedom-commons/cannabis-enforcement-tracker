package research

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
)

func TestV2ImportsOnlyConfiguredResearchCredentials(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local macOS adapter")
	}
	for _, tc := range []struct {
		name, search string
		inlineKey    bool
		want         []string
	}{
		{"default", "", false, []string{"OpenCode Console", "exa"}},
		{"parallel", "parallel", false, []string{"OpenCode Console", "parallel"}},
		{"random", "random", false, []string{"OpenCode Console", "exa", "parallel"}},
		{"inline model key", "random", true, []string{"exa", "parallel"}},
		{"unconnected search", "tavily", false, []string{"OpenCode Console"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			binary := filepath.Join(root, "fake-opencode")
			script := `#!/bin/sh
case "$1" in
 --version) printf 'opencode v2.2.5\n';;
 auth)
  if [ "$2" = export ]; then
   case "$3" in
    'OpenCode Console'|exa|parallel) printf '[{"integrationID":"%s","value":{"type":"api","key":"fixture-private-key"}}]\n' "$3";;
    firecrawl|tavily|tinyfish) printf '[]\n';;
    *) exit 1;;
   esac
  else
   cat >> "$XDG_DATA_HOME/imports.jsonl"
  fi;;
 session) exit 0;;
 *) exit 1;;
esac
`
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			input := filepath.Join(root, "input")
			if err := os.Mkdir(input, 0700); err != nil {
				t.Fatal(err)
			}
			cfg := config.Config{ResearchRunner: "local", OpenCodeBinary: binary, ResearchModel: "opencode-go/deepseek-v4.1-flash", ResearchWebSearch: tc.search}
			if tc.inlineKey {
				path := filepath.Join(root, "provider.json")
				if err := os.WriteFile(path, []byte(`{"provider":{"opencode-go":{"options":{"apiKey":"fixture-model-key"}}}}`), 0600); err != nil {
					t.Fatal(err)
				}
				cfg.OpenCodeConfigFile = path
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd, cleanup, err := (Runner{Config: cfg}).commandArgs(ctx, input, "fixture", []string{"session", "list"})
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			var dataHome string
			for _, env := range cmd.Env {
				if strings.Contains(env, "fixture-private-key") {
					t.Fatal("saved credential leaked into command environment")
				}
				if strings.HasPrefix(env, "XDG_DATA_HOME=") {
					dataHome = strings.TrimPrefix(env, "XDG_DATA_HOME=")
				}
			}
			data, err := os.ReadFile(filepath.Join(dataHome, "imports.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			var imported []string
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var entries []struct{ IntegrationID string }
				if err := json.Unmarshal([]byte(line), &entries); err != nil {
					t.Fatal("invalid imported credential data")
				}
				for _, entry := range entries {
					imported = append(imported, entry.IntegrationID)
				}
			}
			if !reflect.DeepEqual(imported, tc.want) {
				t.Fatalf("imported integrations %v, want %v", imported, tc.want)
			}
		})
	}
}
