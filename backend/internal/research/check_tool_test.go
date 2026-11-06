package research

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckToolServesMCPOnlyWithToken(t *testing.T) {
	var seen Result
	s, err := startCheckServer(func(ctx context.Context, r Result) []string {
		seen = r
		return []string{"figures missing"}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	call := func(token string, body any) (int, map[string]any) {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, s.URL, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	if code, _ := call("wrong", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}); code != http.StatusUnauthorized {
		t.Fatal("accepted a wrong token", code)
	}
	if code, out := call(s.Token, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-03-26"}}); code != 200 || out["result"].(map[string]any)["protocolVersion"] != "2025-03-26" {
		t.Fatal("initialize", code, out)
	}
	if code, _ := call(s.Token, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); code != http.StatusAccepted {
		t.Fatal("notification", code)
	}
	_, out := call(s.Token, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	if !strings.Contains(encoded(out), `"check_result"`) {
		t.Fatal("tool not listed", out)
	}
	proposal := map[string]any{"outcome": "not_relevant", "explanation": "The article is about a traffic accident, not cannabis enforcement."}
	_, out = call(s.Token, map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "check_result", "arguments": map[string]any{"result": proposal}}})
	if !strings.Contains(encoded(out), "figures missing") || seen.Outcome != "not_relevant" {
		t.Fatal("check result not returned", out, seen)
	}
	asText, _ := json.Marshal(proposal)
	_, out = call(s.Token, map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": "check_result", "arguments": map[string]any{"result": string(asText)}}})
	if !strings.Contains(encoded(out), "figures missing") {
		t.Fatal("a result passed as a JSON string was not checked", out)
	}
	_, out = call(s.Token, map[string]any{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": map[string]any{"name": "check_result", "arguments": map[string]any{"result": map[string]any{"outcome": "new_event", "bogus": 1}}}})
	if !strings.Contains(encoded(out), "result parser") {
		t.Fatal("strict parsing skipped", out)
	}
}

func encoded(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestCheckToolConfigurationAllowsOnlyItsServer(t *testing.T) {
	input := t.TempDir()
	if err := os.MkdirAll(filepath.Join(input, ".opencode", "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, ".opencode", "agents", "enforcement-researcher.md"), []byte("---\nmode: primary\npermission:\n  read: allow\n---\nInstructions.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := configureOpenCode2(input, 10); err != nil {
		t.Fatal(err)
	}
	s := &checkServer{URL: "http://127.0.0.1:1234/mcp", Token: "secret"}
	for range 2 { // rewritten on resume without duplicating rules
		if err := addCheckTool(input, s, true); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(input, "opencode.json"))
	var cfg struct {
		MCP struct {
			Servers map[string]struct {
				Type, URL string
				Headers   map[string]string
			}
		}
		Agents map[string]struct{ Permissions []map[string]string }
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	server := cfg.MCP.Servers[CheckToolName]
	if len(cfg.MCP.Servers) != 1 || server.Type != "remote" || server.URL != s.URL || server.Headers["Authorization"] != "Bearer secret" {
		t.Fatal("server not configured", string(b))
	}
	uses := 0
	for _, rule := range cfg.Agents["enforcement-researcher"].Permissions {
		if rule["action"] == "mcp.use" {
			uses++
			if rule["resource"] != CheckToolName {
				t.Fatal("allowed another MCP server", rule)
			}
		}
	}
	if uses != 1 {
		t.Fatal("mcp.use rule missing or duplicated", uses)
	}
	if strings.Contains(string(cfg.Agents["enforcement-probe"].Permissions[0]["action"]), "mcp") {
		t.Fatal("probe gained tool access")
	}
}

func TestResumableSessionMarker(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, ".opencode-session-1")
	for _, d := range []string{home, filepath.Join(dir, ".research-input")} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeMarker(dir, sessionMarker{Home: home, Model: "p/m"}); err != nil {
		t.Fatal(err)
	}
	if !CanResume(dir, "p/m") || CanResume(dir, "p/other") {
		t.Fatal("resume must match the session's model")
	}
	if err := writeMarker(dir, sessionMarker{Home: "/etc", Model: "p/m"}); err != nil {
		t.Fatal(err)
	}
	if CanResume(dir, "p/m") {
		t.Fatal("followed a session home outside its workspace")
	}
	if err := writeMarker(dir, sessionMarker{Home: home, Model: "p/m", Resumes: MaxResumes}); err != nil {
		t.Fatal(err)
	}
	if CanResume(dir, "p/m") {
		t.Fatal("resumed past the limit")
	}
	DiscardSession(dir)
	if HasSession(dir) {
		t.Fatal("marker kept")
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("session home kept", err)
	}
	args := correctionArgs([]string{"run", "--continue", "--agent", "a", "old"}, "new")
	if strings.Count(strings.Join(args, " "), "--continue") != 1 || args[len(args)-1] != "new" {
		t.Fatal(args)
	}
}
