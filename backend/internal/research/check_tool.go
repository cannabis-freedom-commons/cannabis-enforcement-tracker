package research

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/frontmatter"
	"gopkg.in/yaml.v3"
)

// CheckFunc runs the service's own result checks on a proposed answer without publishing anything
// and returns the problems it found, in words the agent can act on. No problems means the answer
// would pass the checks that can run before the final quotation pass.
type CheckFunc func(ctx context.Context, r Result) []string

// CheckToolName is the MCP server name; OpenCode exposes its tool as tracker_check_result.
const CheckToolName = "tracker"

const checkToolDescription = "Check your proposed final JSON result with the tracker service before you answer. " +
	"Pass the complete result object exactly as you would return it. The service runs its schema, " +
	"event-identity, merge-target, report-format and captured-quotation checks and lists every problem. " +
	"Fix them and check again; when it reports no problems, return that same JSON as your final answer. " +
	"Quotations from sources not yet captured are checked later, when the service opens them."

// checkServer is a loopback MCP endpoint (streamable HTTP, JSON responses) that lives for one
// OpenCode invocation. A random bearer token keeps other local processes out.
type checkServer struct {
	URL, Token string
	srv        *http.Server
}

func startCheckServer(check CheckFunc, lookup *reportLookup) (*checkServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		ln.Close()
		return nil, err
	}
	s := &checkServer{URL: "http://" + ln.Addr().String() + "/mcp", Token: hex.EncodeToString(token)}
	s.srv = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { s.serve(w, req, check, lookup) }), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

func (s *checkServer) Close() {
	if s == nil || s.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (s *checkServer) serve(w http.ResponseWriter, req *http.Request, check CheckFunc, lookup *reportLookup) {
	got := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if req.URL.Path != "/mcp" {
		http.NotFound(w, req)
		return
	}
	if req.Method == http.MethodDelete {
		w.WriteHeader(http.StatusOK)
		return
	}
	if req.Method != http.MethodPost {
		// No server-initiated stream: the tool only answers requests.
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 4*1024*1024))
	if err != nil {
		http.Error(w, "unreadable request", http.StatusBadRequest)
		return
	}
	var message rpcRequest
	if err = json.Unmarshal(body, &message); err != nil {
		writeRPC(w, nil, nil, &rpcError{Code: -32700, Message: "parse error"})
		return
	}
	if len(message.ID) == 0 { // a notification
		w.WriteHeader(http.StatusAccepted)
		return
	}
	switch message.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(message.Params, &params)
		if params.ProtocolVersion == "" {
			params.ProtocolVersion = "2025-06-18"
		}
		writeRPC(w, message.ID, map[string]any{"protocolVersion": params.ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": CheckToolName, "version": "1"}}, nil)
	case "ping":
		writeRPC(w, message.ID, map[string]any{}, nil)
	case "tools/list":
		tools := []any{map[string]any{
			"name": "check_result", "description": checkToolDescription,
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"result": map[string]any{"type": "object", "description": "The complete proposed final JSON result."}}, "required": []string{"result"}},
		}}
		if lookup != nil {
			tools = append(tools,
				map[string]any{"name": "find_reports", "description": findReportsDescription, "inputSchema": findReportsSchema},
				map[string]any{"name": "get_report", "description": getReportDescription, "inputSchema": getReportSchema})
		}
		writeRPC(w, message.ID, map[string]any{"tools": tools}, nil)
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err = json.Unmarshal(message.Params, &params); err != nil {
			writeRPC(w, message.ID, nil, &rpcError{Code: -32602, Message: "unknown tool or invalid arguments"})
			return
		}
		var text string
		switch {
		case params.Name == "check_result":
			var args struct {
				Result json.RawMessage `json:"result"`
			}
			if err = json.Unmarshal(params.Arguments, &args); err != nil {
				writeRPC(w, message.ID, nil, &rpcError{Code: -32602, Message: "unknown tool or invalid arguments"})
				return
			}
			ctx, cancel := context.WithTimeout(req.Context(), time.Minute)
			defer cancel()
			text = runCheck(ctx, args.Result, check)
		case params.Name == "find_reports" && lookup != nil:
			text = lookup.find(params.Arguments)
		case params.Name == "get_report" && lookup != nil:
			text = lookup.get(params.Arguments)
		default:
			writeRPC(w, message.ID, nil, &rpcError{Code: -32602, Message: "unknown tool or invalid arguments"})
			return
		}
		writeRPC(w, message.ID, map[string]any{"content": []any{map[string]string{"type": "text", "text": text}}, "isError": false}, nil)
	default:
		writeRPC(w, message.ID, nil, &rpcError{Code: -32601, Message: "method not found"})
	}
}

// runCheck parses the proposal with the same strict parser as the final answer.
func runCheck(ctx context.Context, raw json.RawMessage, check CheckFunc) string {
	data := []byte(raw)
	var quoted string
	if json.Unmarshal(raw, &quoted) == nil { // a model may pass the JSON as a string
		data = []byte(quoted)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return "No result was given. Pass the complete proposed JSON result as the result argument."
	}
	result, err := Strict(data)
	if err != nil {
		return "The result would be rejected by the result parser: " + err.Error() + ". Fix it and check again."
	}
	problems := check(ctx, result)
	if len(problems) == 0 {
		return "No problems found. Return this same JSON result as your final answer."
	}
	return "The service would reject this result. Fix every problem below, then check again:\n- " + strings.Join(problems, "\n- ")
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func writeRPC(w http.ResponseWriter, id json.RawMessage, result any, rpcErr *rpcError) {
	if id == nil {
		id = json.RawMessage("null")
	}
	response := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcErr != nil {
		response["error"] = rpcErr
	} else {
		response["result"] = result
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// addCheckTool points the run's OpenCode configuration at this invocation's check server. It is
// rewritten on every invocation, because a resumed session gets a new port and token.
func addCheckTool(input string, s *checkServer, v2 bool) error {
	path := filepath.Join(input, "opencode.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err = json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	server := map[string]any{"type": "remote", "url": s.URL, "headers": map[string]string{"Authorization": "Bearer " + s.Token}}
	if v2 {
		cfg["mcp"] = map[string]any{"servers": map[string]any{CheckToolName: server}}
		agents, _ := cfg["agents"].(map[string]any)
		if agent, ok := agents["enforcement-researcher"].(map[string]any); ok {
			rules, _ := agent["permissions"].([]any)
			for _, rule := range checkToolRules() {
				if !hasRule(rules, rule) {
					rules = append(rules, rule)
				}
			}
			agent["permissions"] = rules
		}
	} else {
		server["enabled"] = true
		cfg["mcp"] = map[string]any{CheckToolName: server}
		if err = allowLegacyCheckTool(input); err != nil {
			return err
		}
	}
	if data, err = json.Marshal(cfg); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// The research agent may use only this server's tool, on top of its read-only research tools.
func checkToolRules() []map[string]any {
	return []map[string]any{
		{"action": "mcp.use", "resource": CheckToolName, "effect": "allow"},
		{"action": CheckToolName + "_check_result", "resource": "*", "effect": "allow"},
		{"action": CheckToolName + "_*", "resource": "*", "effect": "allow"},
	}
}

func hasRule(rules []any, rule map[string]any) bool {
	for _, existing := range rules {
		if m, ok := existing.(map[string]any); ok && m["action"] == rule["action"] && m["resource"] == rule["resource"] && m["effect"] == rule["effect"] {
			return true
		}
	}
	return false
}

// allowLegacyCheckTool adds the tool to the legacy agent's frontmatter permissions.
func allowLegacyCheckTool(input string) error {
	path := filepath.Join(input, ".opencode", "agents", "enforcement-researcher.md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	parts := frontmatter.Split(string(data))
	if len(parts) != 3 {
		return errors.New("research agent instructions are invalid")
	}
	var header map[string]any
	if err = yaml.Unmarshal([]byte(parts[1]), &header); err != nil {
		return err
	}
	permissions, ok := header["permission"].(map[string]any)
	if !ok {
		return errors.New("research agent permissions are missing")
	}
	for _, tool := range []string{"check_result", "find_reports", "get_report"} {
		permissions[CheckToolName+"_"+tool] = "allow"
	}
	encoded, err := yaml.Marshal(header)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte("---\n"+string(encoded)+"---"+parts[2]), 0644)
}
