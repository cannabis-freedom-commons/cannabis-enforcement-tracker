package research

import (
	"context"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"path/filepath"
	"strings"
	"testing"
)

// The public chat says what each step and tool did, in words, without the step-start noise: what the
// tool was asked and, when it failed, the error as OpenCode gave it.
func TestPublicEventsDescribeStepsAndToolFailures(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "steps.sqlite"))
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
	s.StartRun(ctx, run, "opencode-go/deepseek-v4.1-flash", "local")
	writer := eventWriter{ctx: ctx, store: s, cid: v.CaseID, run: run}
	for _, line := range []string{
		`{"type":"step_start","sessionID":"ses_steps"}`,
		`{"type":"tool_use","part":{"tool":"websearch","state":{"status":"error","error":"search failed for https://secret.example/q?x=1 at /home/runner/job/x.json: rate limited","input":{"query":"private query"}}}}`,
		`{"type":"tool_use","part":{"tool":"codesearch","state":{"status":"completed"}}}`,
		`{"type":"step_finish","part":{"tokens":{"input":14605,"output":1038},"cost":0.01}}`,
	} {
		if _, err := writer.Write([]byte(line + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	activity, err := s.AIActivity(ctx, id, -1)
	if err != nil {
		t.Fatal(err)
	}
	public := store.JSON(activity)
	if strings.Contains(public, "started a model step") {
		t.Fatalf("step-start noise in the chat: %s", public)
	}
	for _, want := range []string{"Searching for corroborating coverage: private query — failed: search failed for https://secret.example/q?x=1 at …/x.json: rate limited.", "Using the codesearch tool — done.", "Step 1 done · 14,605 tokens read, 1,038 written"} {
		if !strings.Contains(public, want) {
			t.Fatalf("missing %q in %s", want, public)
		}
	}
}
