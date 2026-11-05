package research

import (
	"context"
	"encoding/json"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLocalSandboxProtectsHostFilesAndRunsOpenCode(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS local runner")
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
		t.Skip("legacy auth fixture; OpenCode 2 adapter has separate tests")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input")
	os.Mkdir(input, 0755)
	authPath := filepath.Join(root, "auth.json")
	if err = os.WriteFile(authPath, []byte(`{"opencode-go":{"type":"api","key":"fake-selected-provider-key"},"unrelated":{"type":"api","key":"must-never-copy"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	r := Runner{Config: config.Config{ResearchRunner: "local", OpenCodeBinary: binary, OpenCodeAuthFile: authPath, ResearchModel: "opencode-go/deepseek-v4.1-flash", ResearchKey: "private-test-key"}}
	if err = os.WriteFile(filepath.Join(input, "opencode.json"), r.modelConfig(), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd, cleanup, err := r.command(ctx, input, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "XDG_DATA_HOME=") {
			b, err := os.ReadFile(filepath.Join(strings.TrimPrefix(entry, "XDG_DATA_HOME="), "opencode/auth.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), "fake-selected-provider-key") || strings.Contains(string(b), "must-never-copy") {
				t.Fatal("auth was not scoped to the selected provider")
			}
		}
		if strings.HasPrefix(entry, "RESEARCH_API_KEY=") || strings.HasPrefix(entry, "OPENAI_API_KEY=") {
			t.Fatal("local CLI must use saved auth, not provider-key environment overrides")
		}
	}
	cmd.Args = append(cmd.Args[:4], "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sandbox cannot run OpenCode: %s %v", out, err)
	}
	if strings.TrimSpace(string(out)) != version {
		t.Fatal("unexpected harness version")
	}
	probe, release, err := r.command(ctx, input, "config-probe")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	probe.Args = append(probe.Args[:4], "debug", "config")
	configOutput, err := probe.Output()
	if err != nil {
		t.Fatal("isolated OpenCode configuration failed", err)
	}
	var parsed map[string]any
	if err = json.Unmarshal(configOutput, &parsed); err != nil {
		t.Fatal("configuration was not JSON")
	}
	if parsed["provider"] != nil {
		t.Fatal("local OpenCode must use its native provider/auth configuration")
	}
	secret := filepath.Join(root, "host-secret")
	os.WriteFile(secret, []byte("must-not-be-readable"), 0600)
	denied := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", "-f", cmd.Args[2], "/bin/cat", secret)
	denied.Env = cmd.Env
	out, err = denied.CombinedOutput()
	if err == nil || strings.Contains(string(out), "must-not-be-readable") {
		t.Fatal("host file escaped the sandbox")
	}
	for _, v := range cmd.Env {
		if strings.HasPrefix(v, "NOSTR_") || strings.HasPrefix(v, "GITHUB_") || strings.HasPrefix(v, "DATABASE_PATH=") {
			t.Fatal("production authority leaked to OpenCode")
		}
	}
}

func TestModelConfigUsesSelectedModelName(t *testing.T) {
	r := Runner{Config: config.Config{ResearchRunner: "docker", ResearchModel: "opencode-go/another-model"}}
	var cfg struct {
		Provider map[string]struct {
			Models map[string]struct {
				Name string `json:"name"`
			} `json:"models"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(r.modelConfig(), &cfg); err != nil {
		t.Fatal(err)
	}
	if name := cfg.Provider["opencode-go"].Models["another-model"].Name; name != "another-model" {
		t.Fatalf("configured model has wrong name: %q", name)
	}
	message := probeFailure(nil, "other-provider/selected-model#high").Error()
	if !strings.Contains(message, "other-provider/selected-model#high") || strings.Contains(message, "deepseek") {
		t.Fatalf("probe failure must identify selected model: %s", message)
	}
}

func TestCorrectionContinuesSessionWithFeedback(t *testing.T) {
	args := []string{"run", "--standalone", "--agent", "enforcement-researcher", "--format", "json", "Read request.json"}
	got := correctionArgs(args, "fix it")
	want := []string{"run", "--continue", "--standalone", "--agent", "enforcement-researcher", "--format", "json", "fix it"}
	if strings.Join(got, "|") != strings.Join(want, "|") || args[len(args)-1] != "Read request.json" {
		t.Fatal(got)
	}
}
