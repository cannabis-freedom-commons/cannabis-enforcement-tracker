package research

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
)

func providerFixture(t *testing.T, path, key string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"plugin":     []string{"never-copy-this-plugin"},
		"permission": map[string]string{"*": "allow"},
		"provider": map[string]any{
			"custom":    map[string]any{"npm": "@ai-sdk/openai-compatible", "name": "Custom provider", "options": map[string]string{"baseURL": "https://example.org/v1", "apiKey": key}, "models": map[string]any{"custom-model": map[string]any{"name": "Custom model", "limit": map[string]int{"context": 120000, "output": 8000}}}},
			"unrelated": map[string]any{"options": map[string]string{"apiKey": "unrelated-secret"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLocalProviderConfigIsScopedAndPrivate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	source := filepath.Join(root, ".opencode", "opencode.json")
	providerFixture(t, source, "custom-private-key")
	r := Runner{Config: config.Config{ResearchRunner: "local", ResearchModel: "custom/custom-model#high"}}
	home := filepath.Join(root, "session")
	key, err := r.copyLocalProvider(home)
	if err != nil || key != "custom-private-key" {
		t.Fatalf("provider key not loaded: %v", err)
	}
	target := filepath.Join(home, "config", "opencode", "opencode.json")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"custom-private-key", "unrelated-secret", "never-copy-this-plugin", "permission"} {
		if strings.Contains(string(data), value) {
			t.Fatal("unrelated settings or credential written to session config")
		}
	}
	var cfg struct {
		Provider map[string]struct {
			Options map[string]string
			Models  map[string]any
		}
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Provider) != 1 || cfg.Provider["custom"].Options["baseURL"] != "https://example.org/v1" || cfg.Provider["custom"].Models["custom-model"] == nil {
		t.Fatal("custom endpoint/model configuration lost")
	}
	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0600 {
		t.Fatal("provider configuration is not private")
	}
	// Explicit file overrides and provider-scoped environment references work too.
	explicit := filepath.Join(root, "explicit.json")
	providerFixture(t, explicit, "{env:CUSTOM_TEST_PROVIDER_KEY}")
	t.Setenv("CUSTOM_TEST_PROVIDER_KEY", "env-test-key")
	r.Config.OpenCodeConfigFile = explicit
	if key, err = r.copyLocalProvider(home); err != nil || key != "env-test-key" {
		t.Fatalf("explicit configuration did not resolve selected credential: %v", err)
	}
}

func TestLocalProviderConfigErrorsDoNotExposeContents(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "broken.json")
	os.WriteFile(source, []byte(`{"provider":{"custom":{"options":{"apiKey":"private-value"}}},broken`), 0600)
	r := Runner{Config: config.Config{ResearchModel: "custom/model", OpenCodeConfigFile: source}}
	_, err := r.copyLocalProvider(filepath.Join(root, "session"))
	if err == nil || strings.Contains(err.Error(), "private-value") {
		t.Fatal("invalid config not rejected privately")
	}
}

func v2ProviderFixture(t *testing.T, path string) {
	t.Helper()
	// Comments, trailing commas and URL/comment-like string contents are deliberate.
	data := `{
		// Native OpenCode 2 config.
		"plugins": ["never-copy-this-plugin"],
		"permissions": [{"action":"*","resource":"*","effect":"allow"}],
		"providers": {
			"custom": {
				"name": "Custom provider",
				"env": ["CUSTOM_TEST_PROVIDER_KEY"],
				"package": "@opencode/ai/providers/openai-compatible",
				"settings": {"baseURL": "https://example.org/v1", "apiKey": "fixture-v2-private-key",},
				"body": {"metadata": {"note": "Keep // and /* inside strings, with \"quotes\"."}},
				"models": {
					"custom-model": {
						"name": "Custom model",
						"limit": {"context": 120000, "output": 8000},
						"variants": [{"id": "max", "body": {"reasoning_effort": "max"}},],
					},
				},
			},
			/* Never import other providers. */
			"unrelated": {"settings": {"apiKey": "unrelated-secret"}},
		},
	}`
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLocalV2ProviderFromGlobalJSONCIsScopedAndPrivate(t *testing.T) {
	for _, location := range []string{"global json", "global jsonc", "xdg jsonc", "explicit jsonc"} {
		t.Run(location, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("XDG_CONFIG_HOME", "")
			// A native v2 definition overrides the legacy provider without mixing schemas.
			providerFixture(t, filepath.Join(root, ".opencode", "opencode.json"), "legacy-private-key")
			source := filepath.Join(root, ".config", "opencode", "opencode.jsonc")
			r := Runner{Config: config.Config{ResearchModel: "custom/custom-model#max"}}
			switch location {
			case "global json":
				source = filepath.Join(root, ".config", "opencode", "opencode.json")
			case "xdg jsonc":
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
				source = filepath.Join(root, "xdg", "opencode", "opencode.jsonc")
			case "explicit jsonc":
				source = filepath.Join(root, "explicit.jsonc")
				r.Config.OpenCodeConfigFile = source
			}
			v2ProviderFixture(t, source)
			home := filepath.Join(root, "session")
			key, err := r.copyLocalProvider(home)
			if err != nil || key != "fixture-v2-private-key" {
				t.Fatalf("native provider credential not loaded: %v", err)
			}
			target := filepath.Join(home, "config", "opencode", "opencode.json")
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"fixture-v2-private-key", "legacy-private-key", "unrelated-secret", "never-copy-this-plugin", "permissions"} {
				if strings.Contains(string(data), forbidden) {
					t.Fatal("unrelated settings or credential escaped provider selection")
				}
			}
			var cfg struct {
				Provider  map[string]any
				Providers map[string]struct {
					Package  string
					Env      []string
					Settings map[string]string
					Body     struct{ Metadata map[string]string }
					Models   map[string]struct {
						Variants []struct {
							ID   string
							Body map[string]string
						}
					}
				}
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatal(err)
			}
			provider := cfg.Providers["custom"]
			if cfg.Provider != nil || len(cfg.Providers) != 1 || provider.Package != "@opencode/ai/providers/openai-compatible" || provider.Settings["baseURL"] != "https://example.org/v1" || provider.Settings["apiKey"] != "{env:"+providerKeyEnv+"}" || len(provider.Env) != 1 || provider.Env[0] != providerKeyEnv {
				t.Fatal("native provider routing or private credential reference lost")
			}
			variants := provider.Models["custom-model"].Variants
			if len(variants) != 1 || variants[0].ID != "max" || variants[0].Body["reasoning_effort"] != "max" || provider.Body.Metadata["note"] != `Keep // and /* inside strings, with "quotes".` {
				t.Fatal("JSONC parsing changed model variants or string contents")
			}
			info, err := os.Stat(target)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("native provider configuration is not private")
			}
		})
	}
}

func TestLocalProviderConfigMergesGlobalSettingsAndResolvesDeclaredCredential(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CUSTOM_TEST_PROVIDER_KEY", "declared-private-key")
	path := filepath.Join(root, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"providers":{"custom":{"env":["UNSET_TEST_PROVIDER_KEY","CUSTOM_TEST_PROVIDER_KEY"],"settings":{"baseURL":"https://example.org/v1"},"models":{"first":{}}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+"c", []byte(`{"providers":{"custom":{"settings":{"timeout":30000},"models":{"second":{"variants":[{"id":"max"}]}}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNSET_TEST_PROVIDER_KEY", "")
	home := filepath.Join(root, "session")
	r := Runner{Config: config.Config{ResearchModel: "custom/second#max"}}
	key, err := r.copyLocalProvider(home)
	if err != nil || key != "declared-private-key" {
		t.Fatalf("declared provider credential not resolved: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, "config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Providers map[string]struct {
			Settings map[string]any
			Models   map[string]any
		}
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	p := cfg.Providers["custom"]
	if len(p.Models) != 2 || p.Settings["baseURL"] != "https://example.org/v1" || p.Settings["timeout"] != float64(30000) {
		t.Fatal("global provider settings were replaced instead of merged")
	}
	// An explicit configuration remains authoritative, including a missing file.
	r.Config.OpenCodeConfigFile = filepath.Join(root, "missing.jsonc")
	if _, err := r.copyLocalProvider(home); err == nil {
		t.Fatal("missing explicit configuration silently fell back to global settings")
	}
}

func TestNativeLocalHarnessLoadsCustomProviderFromHomeConfig(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS sandbox")
	}
	binary, err := exec.LookPath("opencode")
	if err != nil {
		t.Skip("OpenCode not installed")
	}
	version, err := localVersion(context.Background(), binary)
	if err != nil {
		t.Fatal(err)
	}
	if openCode2(version) {
		t.Skip("legacy debug config command; v2 tested with fake harness")
	}
	root := t.TempDir()
	source := filepath.Join(root, "provider.json")
	providerFixture(t, source, "custom-private-key")
	input := filepath.Join(root, "input")
	os.MkdirAll(input, 0700)
	r := Runner{Config: config.Config{ResearchRunner: "local", OpenCodeBinary: binary, ResearchModel: "custom/custom-model", OpenCodeConfigFile: source, OpenCodeAuthFile: filepath.Join(root, "missing-auth.json")}}
	os.WriteFile(filepath.Join(input, "opencode.json"), r.modelConfig(), 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd, cleanup, err := r.command(ctx, input, "provider-config-test")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	cmd.Args = append(cmd.Args[:4], "debug", "config")
	data, err := cmd.Output()
	if err != nil {
		t.Fatal("native CLI could not load isolated configuration", err)
	}
	var cfg struct {
		Provider map[string]struct {
			Options map[string]string
			Models  map[string]any
		}
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal("native configuration was not JSON")
	}
	if cfg.Provider["custom"].Options["baseURL"] != "https://example.org/v1" || cfg.Provider["custom"].Options["apiKey"] != "custom-private-key" || cfg.Provider["custom"].Models["custom-model"] == nil {
		t.Fatal("native harness did not resolve custom provider endpoint, model and credentials")
	}
	if cfg.Provider["unrelated"].Options != nil {
		t.Fatal("unrelated provider escaped selection")
	}
	if selectedProviderKey(cmd) != "custom-private-key" {
		t.Fatal("selected key missing for diagnostics redaction")
	}
}
