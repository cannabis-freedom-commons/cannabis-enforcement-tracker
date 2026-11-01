package research

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func localVersion(ctx context.Context, binary string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New("OpenCode binary is not executable")
	}
	return strings.TrimSpace(string(out)), nil
}
func openCode2(version string) bool {
	return strings.HasPrefix(strings.TrimPrefix(strings.TrimSpace(version), "opencode "), "v2.") || strings.HasPrefix(strings.TrimSpace(version), "2.")
}
func v2Args(args []string) []string {
	out := []string{}
	for _, arg := range args {
		if arg == "--pure" {
			continue
		}
		out = append(out, arg)
		if arg == "run" {
			out = append(out, "--standalone")
		}
	}
	return out
}

// Import only the model and configured search integrations. The research server
// has its own credential store, so the user's /connect state is otherwise absent.
func (r Runner) importOpenCode2Auth(ctx context.Context, binary string, command *exec.Cmd) error {
	targets := []string{}
	if selectedProviderKey(command) == "" {
		target := strings.SplitN(r.Config.ResearchModel, "/", 2)[0]
		if target == "opencode-go" || target == "opencode" {
			target = "OpenCode Console"
		}
		targets = append(targets, target)
	}
	search := r.Config.ResearchWebSearch
	if search == "" {
		search = "exa"
	}
	if search == "random" {
		targets = append(targets, "exa", "parallel", "firecrawl", "tavily", "tinyfish")
	} else {
		targets = append(targets, search)
	}
	for _, target := range targets {
		if err := importOpenCode2Credential(ctx, binary, command, target); err != nil {
			return err
		}
	}
	return nil
}

// Native scoped exports keep credentials out of environment variables, evidence,
// and logs. An unconnected search integration exports an empty list.
func importOpenCode2Credential(ctx context.Context, binary string, command *exec.Cmd, target string) error {
	export := exec.CommandContext(ctx, binary, "auth", "export", target)
	// This short native credential read uses the user's normal OpenCode auth. None
	// of the exported bytes or command diagnostics are public or written to logs.
	export.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + userHome()}
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		if v := os.Getenv(name); v != "" {
			export.Env = append(export.Env, name+"="+v)
		}
	}
	secret, err := export.Output()
	if err != nil {
		return errors.New("OpenCode could not load the selected saved integration credentials")
	}
	defer clear(secret)
	if len(secret) > 1024*1024 || !json.Valid(secret) {
		return errors.New("OpenCode credential export was invalid")
	}
	if bytes.Equal(bytes.TrimSpace(secret), []byte("[]")) {
		return nil
	}
	importer := exec.CommandContext(ctx, command.Path, append(append([]string(nil), command.Args[1:4]...), "auth", "import", "--standalone")...)
	importer.Env = command.Env
	importer.Dir = command.Dir
	importer.Stdin = bytes.NewReader(secret)
	importer.Stdout = &capped{Max: 64 * 1024}
	importer.Stderr = &capped{Max: 64 * 1024}
	if err = importer.Run(); err != nil {
		return errors.New("OpenCode could not import its saved integration into the isolated session")
	}
	return nil
}
func userHome() string { home, _ := os.UserHomeDir(); return home }

// Write native v2 configuration only in the disposable evidence snapshot.
func configureOpenCode2(input string, steps int, toolOutput ...string) error {
	if steps <= 0 {
		steps = 24
	}
	deny := []map[string]string{{"action": "*", "resource": "*", "effect": "deny"}}
	rules := append([]map[string]string(nil), deny...)
	for _, action := range []string{"read", "glob", "grep", "skill", "webfetch", "websearch"} {
		rules = append(rules, map[string]string{"action": action, "resource": "*", "effect": "allow"})
	}
	// Large tool results spill into the isolated session's XDG data directory,
	// outside the read-only evidence input. Only that generated folder is external context.
	for _, directory := range toolOutput {
		rules = append(rules, map[string]string{"action": "external_directory", "resource": filepath.ToSlash(filepath.Clean(directory)) + "/*", "effect": "allow"})
	}
	agents := map[string]any{"enforcement-probe": map[string]any{
		"description": "Short connectivity check", "mode": "primary", "steps": 2,
		"permissions": deny, "system": probeInstruction}}
	path := filepath.Join(input, ".opencode", "agents", "enforcement-researcher.md")
	if data, err := os.ReadFile(path); err == nil {
		parts := strings.SplitN(string(data), "---", 3)
		if len(parts) != 3 {
			return errors.New("research agent instructions are invalid")
		}
		agents["enforcement-researcher"] = map[string]any{"description": "Research enforcement sources", "mode": "primary", "steps": steps, "permissions": rules, "system": strings.TrimSpace(parts[2])}
		// Avoid merging deprecated agent frontmatter into the native v2 definition.
		if err = os.Remove(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	cfg := map[string]any{"$schema": "https://opencode.ai/config.json", "snapshots": false, "permissions": deny, "agents": agents}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(input, "opencode.json"), data, 0644)
}
