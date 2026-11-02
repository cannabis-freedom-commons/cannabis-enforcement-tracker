package research

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const probePrompt = "Reply exactly OK."
const probeInstruction = "Reply exactly OK to the connectivity test. Do not use tools."

// Local operator diagnostics are deliberately excluded from public health JSON.
type ProbeDiagnostics struct {
	Attempts         []ProbeAttempt `json:"attempts,omitempty"`
	Model            string         `json:"model"`
	Version          string         `json:"version"`
	Prompt           string         `json:"prompt"`
	AgentInstruction string         `json:"agent_instruction"`
	Arguments        []string       `json:"arguments"`
	Response         string         `json:"response"`
	Stdout           string         `json:"stdout"`
	Stderr           string         `json:"stderr"`
	ExitCode         *int           `json:"exit_code,omitempty"`
	DurationMS       int64          `json:"duration_ms"`
	Error            string         `json:"error,omitempty"`
	File             string         `json:"-"`
}

var probeSecretPattern = regexp.MustCompile(`(?i)(bearer\s+[^\s"'\\]+|(?:sk|oc|opencode)[-_][A-Za-z0-9_-]{16,})`)
var probeCredentialPattern = regexp.MustCompile(`(?i)("(?:api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|password)"\s*:\s*")[^"]*(")`)

func (r Runner) redactProbeText(value string, extraSecrets ...string) string {
	for _, secret := range append([]string{r.Config.ResearchKey, r.Config.IngestToken, r.Config.BotSecret, r.Config.GatewaySecret, r.Config.WebhookSecret}, extraSecrets...) {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	value = probeSecretPattern.ReplaceAllString(value, "[redacted]")
	return probeCredentialPattern.ReplaceAllString(value, `${1}[redacted]${2}`)
}
func (d *ProbeDiagnostics) save(root string) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(root, ".startup-diagnostics-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	target := filepath.Join(root, "startup-check.json")
	if err = os.Rename(file.Name(), target); err != nil {
		return err
	}
	d.File = target
	return nil
}

type ProbeAttempt struct {
	Response   string `json:"response"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

func (d ProbeDiagnostics) attempt() ProbeAttempt {
	return ProbeAttempt{d.Response, d.Stdout, d.Stderr, d.ExitCode, d.DurationMS, d.Error}
}
