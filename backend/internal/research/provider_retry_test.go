package research

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// A provider error that can pass (HTTP 404 "unknown model" after a step) does not end the run: the
// same session continues where it stopped. Refused credentials end it at once.
func TestProviderErrorContinuesTheSameSession(t *testing.T) {
	old := providerRetryWait
	providerRetryWait = time.Millisecond
	defer func() { providerRetryWait = old }()

	for _, tc := range []struct {
		name, errorType string
		status, calls   int
	}{
		{"unknown model", "provider.invalid-request", 404, 2},
		{"credentials", "provider.auth", 401, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			s, err := store.Open(filepath.Join(dir, "runs.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.DB.Close()
			id, err := s.Accept(ctx, store.Intake{Origin: "web", URL: "https://news.example/a", CanonicalURL: "https://news.example/a"})
			if err != nil {
				t.Fatal(err)
			}
			v, _ := s.Get(ctx, id)
			run, err := s.Reserve(ctx, v.CaseID, "", 100, 1000)
			if err != nil {
				t.Fatal(err)
			}
			s.StartRun(ctx, run, "provider/model", "local")

			// the first call takes a step and fails on the provider; a later one answers with notes
			calls := filepath.Join(dir, "calls")
			script := filepath.Join(dir, "opencode")
			body := `#!/bin/sh
echo "$*" >> "` + calls + `"
if [ "$(wc -l < "` + calls + `")" -eq 1 ]; then
  echo '{"type":"step_start","sessionID":"ses_1"}'
  echo '{"type":"step_finish","part":{"tokens":{"input":1,"output":1}}}'
  echo '{"type":"error","error":{"type":"` + tc.errorType + `","status":` + itoa(tc.status) + `,"message":"unknown model"}}'
  exit 1
fi
echo '{"type":"step_start","sessionID":"ses_1"}'
echo '{"type":"text","part":{"text":"notes"}}'
echo '{"type":"step_finish","part":{"tokens":{"input":1,"output":1}}}'
`
			if err := os.WriteFile(script, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			r := Runner{Config: config.Config{ResearchRunner: "local", ResearchTimeout: time.Minute, ResearchMaxSteps: 24}, Store: s}
			cmd := exec.CommandContext(ctx, script, "run", "--agent", "enforcement-researcher", "--format", "json", "Read request.json")
			_, err = r.execute(ctx, v.CaseID, dir, run, cmd, false)
			raw, _ := os.ReadFile(calls)
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if len(lines) != tc.calls {
				t.Fatalf("%d OpenCode calls, want %d: %q (%v)", len(lines), tc.calls, lines, err)
			}
			if tc.calls == 2 {
				if !strings.HasPrefix(lines[1], "run --continue ") || !strings.Contains(lines[1], "Continue from where you stopped") {
					t.Fatalf("the retry must continue the session: %q", lines[1])
				}
				if err == nil || strings.Contains(err.Error(), "exited with code") {
					t.Fatalf("the run must end on the continued answer, not the provider error: %v", err)
				}
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
