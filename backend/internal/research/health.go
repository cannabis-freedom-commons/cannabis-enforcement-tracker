package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type HealthSnapshot struct {
	Diagnostics *ProbeDiagnostics `json:"-"`
	Status      string            `json:"status"`
	Message     string            `json:"message"`
	Version     string            `json:"version,omitempty"`
	CheckedAt   int64             `json:"checked_at"`
}
type Health struct {
	mu       sync.RWMutex
	snapshot HealthSnapshot
}

func NewHealth() *Health {
	return &Health{snapshot: HealthSnapshot{Status: "checking", Message: "Checking the OpenCode binary and a short model test prompt."}}
}
func (h *Health) Snapshot() HealthSnapshot { h.mu.RLock(); defer h.mu.RUnlock(); return h.snapshot }
func (h *Health) Ready() bool              { return h != nil && h.Snapshot().Status == "ready" }
func (h *Health) Set(s HealthSnapshot)     { h.mu.Lock(); h.snapshot = s; h.mu.Unlock() }

// Probe invokes the same actual OpenCode harness/model/auth path used by research,
// with a neutral workspace and a short no-tool agent instead of a real report.
func (r Runner) Probe(ctx context.Context) (string, error) {
	diagnostics, err := r.ProbeDetails(ctx)
	return diagnostics.Version, err
}

var errEmptyProbeReply = errors.New("OpenCode completed the model call without an assistant reply")

func (r Runner) ProbeDetails(ctx context.Context) (ProbeDiagnostics, error) {
	first, err := r.probeAttempt(ctx)
	first.Attempts = []ProbeAttempt{first.attempt()}
	result := first
	if errors.Is(err, errEmptyProbeReply) && ctx.Err() == nil {
		result, err = r.probeAttempt(ctx)
		result.Attempts = []ProbeAttempt{first.attempt(), result.attempt()}
		result.DurationMS += first.DurationMS
		if result.File == "" {
			result.File = first.File
		}
	}
	if result.File != "" {
		_ = result.save(filepath.Dir(result.File))
	}
	return result, err
}
func (r Runner) probeAttempt(ctx context.Context) (diagnostics ProbeDiagnostics, probeErr error) {
	diagnostics = ProbeDiagnostics{Model: r.Config.ResearchModel, Prompt: probePrompt, AgentInstruction: probeInstruction}
	started := time.Now()
	var diagnosticRoot string
	var providerKey string
	defer func() {
		diagnostics.DurationMS = time.Since(started).Milliseconds()
		diagnostics.Response = r.redactProbeText(diagnostics.Response, providerKey)
		diagnostics.Stdout = r.redactProbeText(diagnostics.Stdout, providerKey)
		diagnostics.Stderr = r.redactProbeText(diagnostics.Stderr, providerKey)
		if probeErr != nil {
			diagnostics.Error = r.redactProbeText(probeErr.Error(), providerKey)
		}
		if diagnosticRoot != "" {
			if err := diagnostics.save(diagnosticRoot); err != nil {
				diagnostics.Error += " (startup diagnostics file could not be written)"
			}
		}
	}()

	if r.Config.ResearchRunner == "local" {
		name := r.Config.OpenCodeBinary
		if name == "" {
			name = "opencode"
		}
		if _, err := exec.LookPath(name); err != nil {
			return diagnostics, errors.New("OpenCode binary is not accessible; install it or set OPENCODE_BIN")
		}
	}
	root, err := filepath.Abs(r.Config.Workspaces)
	if err != nil {
		return diagnostics, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return diagnostics, err
	}
	diagnosticRoot = root
	dir, err := os.MkdirTemp(root, ".startup-check-")
	if err != nil {
		return diagnostics, err
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "input")
	if err = os.Mkdir(input, 0755); err != nil {
		return diagnostics, err
	}
	if err = os.WriteFile(filepath.Join(input, "opencode.json"), r.modelConfig(), 0644); err != nil {
		return diagnostics, err
	}
	args := []string{"run", "--pure", "--agent", "enforcement-probe", "--model", r.Config.ResearchModel, "--format", "json", probePrompt}
	command, cleanup, err := r.commandArgs(ctx, input, "startup-check", args)
	if err != nil {
		return diagnostics, err
	}
	defer cleanup()
	providerKey = selectedProviderKey(command)
	diagnostics.Arguments = append([]string(nil), args...)
	if r.Config.ResearchRunner == "local" && strings.Contains(strings.Join(command.Args, " "), "--standalone") {
		diagnostics.Arguments = v2Args(args)
	}
	version := "container"
	diagnostics.Version = version
	if r.Config.ResearchRunner == "local" {
		probe := exec.CommandContext(ctx, command.Path, append(append([]string(nil), command.Args[1:4]...), "--version")...)
		probe.Env = command.Env
		probe.Dir = command.Dir
		b, err := probe.Output()
		if err != nil {
			return diagnostics, errors.New("OpenCode binary could not run inside its filesystem sandbox")
		}
		version = strings.TrimSpace(string(b))
		diagnostics.Version = version

	}
	out := &capped{Max: 1024 * 1024}
	stderr := &capped{Max: 256 * 1024}
	command.Stdout = out
	command.Stderr = stderr
	runErr := command.Run()
	diagnostics.Stdout = out.String()
	diagnostics.Stderr = stderr.String()
	if command.ProcessState != nil {
		code := command.ProcessState.ExitCode()
		diagnostics.ExitCode = &code
	}
	if runErr != nil {
		return diagnostics, probeFailure(out.Bytes(), r.Config.ResearchModel)
	}
	text, err := FinalText(out.Bytes())
	if err != nil {
		if err.Error() == "missing final result" {
			return diagnostics, errEmptyProbeReply
		}
		return diagnostics, probeFailure(out.Bytes(), r.Config.ResearchModel)
	}
	diagnostics.Response = string(text)
	if !probeReplyUsable(text) {
		return diagnostics, errEmptyProbeReply
	}
	return diagnostics, nil
}

// Readiness checks a completed model call, not exact wording. Provider/harness
// preambles and summaries must not turn a working model into a startup failure.
func probeReplyUsable(text []byte) bool { return strings.TrimSpace(string(text)) != "" }
func probeFailure(data []byte, model string) error {
	// Never expose provider response bodies or credentials from the execution stream.
	for _, line := range strings.Split(string(data), "\n") {
		var event struct {
			Type  string `json:"type"`
			Error struct {
				Name string `json:"name"`
				Data struct {
					Status int `json:"statusCode"`
				} `json:"data"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(line), &event) == nil && event.Type == "error" {
			if event.Error.Data.Status == 401 || event.Error.Data.Status == 403 || strings.Contains(strings.ToLower(event.Error.Name), "auth") {
				return errors.New("OpenCode could not authenticate the selected provider; connect it in OpenCode and restart the backend")
			}
		}
	}
	return fmt.Errorf("OpenCode model test failed; verify opencode run --model %q in your terminal, then restart the backend", model)
}
func (r Runner) Check(ctx context.Context, h *Health) {
	check, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	diagnostics, err := r.ProbeDetails(check)
	state := HealthSnapshot{Status: "ready", Message: "OpenCode binary and model test passed. Queued requests are processed automatically.", Version: diagnostics.Version, Diagnostics: &diagnostics, CheckedAt: time.Now().Unix()}
	if err != nil {
		state.Status = "failed"
		state.Message = err.Error()
	}
	h.Set(state)
	if r.Store != nil {
		_, _ = r.Store.DB.ExecContext(ctx, "UPDATE submission_feed_version SET version=version+1 WHERE singleton=1")
	}
}
