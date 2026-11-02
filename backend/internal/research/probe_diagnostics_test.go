package research

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
)

func TestFailedProbeRetainsPromptResponseAndPrivateRedactedOutput(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS local harness")
	}
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", "")
	v2ProviderFixture(t, filepath.Join(root, ".config", "opencode", "opencode.jsonc"))
	binary := filepath.Join(root, "fake-opencode")
	script := `#!/bin/sh
case "$1" in
 --version) printf 'opencode v2.2.0\n';;
 auth) if [ "$2" = export ]; then printf '[{"credential":{"value":"fixture-auth-secret"}}]\n'; else cat >/dev/null; fi;;
 run) printf '{"type":"text","part":{"text":"Unexpected answer %s"}}\n' "$TRACKER_OPENCODE_PROVIDER_KEY"; printf 'Provider diagnostic fixture-provider-secret\n' >&2; exit 7;;
 *) exit 1;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	r := Runner{Config: config.Config{ResearchRunner: "local", OpenCodeBinary: binary, ResearchModel: "custom/custom-model#max", ResearchKey: "fixture-provider-secret", Workspaces: filepath.Join(root, "jobs")}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	health := NewHealth()
	r.Check(ctx, health)
	snapshot := health.Snapshot()
	if snapshot.Status != "failed" || snapshot.Diagnostics == nil {
		t.Fatal(snapshot)
	}
	d := snapshot.Diagnostics
	if d.Prompt != probePrompt || d.AgentInstruction != probeInstruction || !strings.Contains(d.Stdout, "Unexpected answer [redacted]") || d.Stderr != "Provider diagnostic [redacted]\n" || d.ExitCode == nil || *d.ExitCode != 7 || d.Error == "" {
		t.Fatal(d)
	}
	if !strings.Contains(strings.Join(d.Arguments, " "), "--standalone") || strings.Contains(strings.Join(d.Arguments, " "), "--pure") {
		t.Fatal(d.Arguments)
	}
	data, err := os.ReadFile(d.File)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fixture-provider-secret", "fixture-auth-secret", "fixture-v2-private-key"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("credential saved in diagnostics")
		}
	}
	var saved ProbeDiagnostics
	if err = json.Unmarshal(data, &saved); err != nil || saved.Stdout != d.Stdout || saved.Stdout == "" {
		t.Fatal("missing full saved response", err)
	}
	stat, _ := os.Stat(d.File)
	if stat.Mode().Perm() != 0600 {
		t.Fatal("diagnostics are not private", stat.Mode())
	}
	public, _ := json.Marshal(snapshot)
	for _, private := range []string{probePrompt, "Unexpected answer", "Provider diagnostic", d.File, "agent_instruction", "stdout"} {
		if strings.Contains(string(public), private) {
			t.Fatal("operator diagnostics leaked into public health JSON", private)
		}
	}
}
func TestProbeDiagnosticsRedactCredentialFields(t *testing.T) {
	r := Runner{}
	input := `{"api_key":"provider-secret","authorization":"Bearer confidential-token","access_token":"token","text":"An ordinary model reply"}`
	out := r.redactProbeText(input)
	if !json.Valid([]byte(out)) {
		t.Fatal("redaction corrupted structured output", out)
	}
	for _, secret := range []string{"provider-secret", "confidential-token", `"token"`} {
		if strings.Contains(out, secret) {
			t.Fatal(out)
		}
	}
	if !strings.Contains(out, "ordinary model reply") {
		t.Fatal("lost diagnostic content", out)
	}
}

func TestProbeRetriesAnEmptySuccessfulStopOnce(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS local harness")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "fake-opencode")
	counter := filepath.Join(root, "calls")
	script := `#!/bin/sh
case "$1" in
 --version) printf 'opencode v2.2.0\n';;
 auth) if [ "$2" = export ]; then
   if [ "$3" != 'OpenCode Console' ]; then printf '[]\n'; exit 0; fi
   n=0; if [ -f '` + counter + `' ]; then n=$(cat '` + counter + `'); fi
   n=$((n+1)); printf '%s' "$n" > '` + counter + `'; printf '{"attempt":%s}' "$n"
  else cat > "$HOME/attempt.json"; fi;;
 run) if grep -q '"attempt":1' "$HOME/attempt.json"; then
   printf '{"type":"step_finish","part":{"reason":"stop","tokens":{"output":1}}}\n'
  else printf '{"type":"text","part":{"text":"OK"}}\n'; fi;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	runner := Runner{Config: config.Config{ResearchRunner: "local", OpenCodeBinary: binary, ResearchModel: "opencode-go/deepseek-v4.1-flash#max", Workspaces: filepath.Join(root, "jobs")}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := runner.ProbeDetails(ctx)
	if err != nil || d.Response != "OK" || len(d.Attempts) != 2 || d.Attempts[0].Error == "" || d.Attempts[1].Error != "" {
		t.Fatal(d, err)
	}
	data, err := os.ReadFile(d.File)
	if err != nil {
		t.Fatal(err)
	}
	var saved ProbeDiagnostics
	if err = json.Unmarshal(data, &saved); err != nil || len(saved.Attempts) != 2 {
		t.Fatal("retry diagnostics not preserved", err)
	}
}
