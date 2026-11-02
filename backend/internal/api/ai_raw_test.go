package api

import (
	"context"
	"encoding/json"
	"github.com/klauspost/compress/zstd"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func TestRawAILogIncludesEmittedReasoningToolsAndLivePartialOutput(t *testing.T) {
	s, _ := server(t)
	s.Config.Workspaces = t.TempDir()
	s.Config.ResearchKey = "fixture-provider-credential"
	id, err := s.Store.Accept(context.Background(), store.Intake{Origin: "web", URL: "https://news.example/raw", CanonicalURL: "https://news.example/raw"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := s.Store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(s.Config.Workspaces, "run")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	run, err := s.Store.Reserve(context.Background(), value.CaseID, workspace, 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	stream := `{"type":"reasoning","part":{"text":"Fixture reasoning emitted by the external model"}}
{"type":"tool_use","part":{"tool":"read","state":{"input":{"path":"request.json"},"output":"Fixture full tool result","api_key":"other-secret"}}}
{"type":"text","part":{"text":"fixture-provider-credential"}}
{"type":"text","part":`
	if err = os.WriteFile(filepath.Join(workspace, "research-events.jsonl"), []byte(stream), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(workspace, "research-stderr.log"), []byte("diagnostic fixture-provider-credential"), 0600); err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	if response := request(h, "GET", "/v1/submissions/"+id+"/ai/raw", "", ""); response.Code != 404 {
		t.Fatal("raw route enabled by default")
	}
	s.Config.RawAILogs = true
	response := request(h, "GET", "/v1/submissions/"+id+"/ai/raw", "", "")
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var result struct {
		RawRuns []rawAIRun `json:"raw_runs"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.RawRuns) != 1 || result.RawRuns[0].RunID != run || len(result.RawRuns[0].Events) != 3 || result.RawRuns[0].PartialLine == "" || !result.RawRuns[0].Available {
		t.Fatal("raw capture truncated")
	}
	body := response.Body.String()
	for _, expected := range []string{"Fixture reasoning emitted", "Fixture full tool result", "request.json", "diagnostic [redacted]"} {
		if !strings.Contains(body, expected) {
			t.Fatal("missing", expected)
		}
	}
	for _, private := range []string{"fixture-provider-credential", "other-secret", workspace} {
		if strings.Contains(body, private) {
			t.Fatal("credential or host path leaked")
		}
	}
	// The public JSON route still contains none of the raw event content.
	public := request(h, "GET", "/v1/submissions/"+id+"/ai/log", "", "")
	if strings.Contains(public.Body.String(), "Fixture reasoning") || strings.Contains(public.Body.String(), "Fixture full tool result") {
		t.Fatal("public transcript changed")
	}
	encoder, _ := zstd.NewWriter(nil)
	os.WriteFile(filepath.Join(workspace, "research-events.jsonl.zst"), encoder.EncodeAll([]byte(stream), nil), 0600)
	encoder.Close()
	os.Remove(filepath.Join(workspace, "research-events.jsonl"))
	archived := request(h, "GET", "/v1/submissions/"+id+"/ai/raw", "", "")
	if archived.Code != 200 || archived.Body.String() != body {
		t.Fatal("compressed raw log response changed", archived.Code)
	}
	if _, _, err := s.rawLogFile(workspace, "research-events.jsonl", 10); err == nil {
		t.Fatal("decompressed log bypassed size bound")
	}
	os.Remove(filepath.Join(workspace, "research-events.jsonl.zst"))
	// Removing an archived file produces an unavailable entry rather than invented data.
	os.Remove(filepath.Join(workspace, "research-events.jsonl"))
	response = request(h, "GET", "/v1/submissions/"+id+"/ai/raw", "", "")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"available":false`) {
		t.Fatal(response.Body.String())
	}
}
func TestRawLogFilesStayInsideJobStorage(t *testing.T) {
	s, _ := server(t)
	s.Config.Workspaces = t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "research-events.jsonl"), []byte("outside"), 0600)
	if _, _, err := s.rawLogFile(outside, "research-events.jsonl", 100); err == nil {
		t.Fatal("read outside workspace")
	}
	inside := filepath.Join(s.Config.Workspaces, "run")
	os.Mkdir(inside, 0700)
	if err := os.Symlink(filepath.Join(outside, "research-events.jsonl"), filepath.Join(inside, "research-events.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.rawLogFile(inside, "research-events.jsonl", 100); err == nil {
		t.Fatal("symlink escaped job storage")
	}
}
