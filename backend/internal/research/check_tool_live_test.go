package research

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
)

// Opt-in: TRACKER_LIVE_OPENCODE_MODEL=provider/model go test ./internal/research -run Live
// runs the real local OpenCode in its sandbox against the loopback check tool. Run it from a
// normal terminal: OpenCode's own sandbox cannot start inside another sandbox.
func TestLiveAgentUsesCheckTool(t *testing.T) {
	model := os.Getenv("TRACKER_LIVE_OPENCODE_MODEL")
	if model == "" {
		t.Skip("set TRACKER_LIVE_OPENCODE_MODEL to run against a real model")
	}
	// A real workspace checkout, as the service makes, outside the sandbox-denied temp folders.
	base, err := filepath.Abs(envOr("TRACKER_LIVE_ROOT", ".live-test"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(base)
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	root, err := (Runner{Config: config.Config{Workspaces: base, RepoPath: repo}}).Snapshot(context.Background(), "live")
	if err != nil {
		t.Fatal(err)
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, ".research-input")
	if err = os.MkdirAll(filepath.Join(input, ".opencode", "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	agent := "---\ndescription: test\nmode: primary\nsteps: 8\npermission:\n  \"*\": deny\n  read: allow\n---\nYou test a tool. Follow the user's message exactly.\n"
	if err = os.WriteFile(filepath.Join(input, ".opencode", "agents", "enforcement-researcher.md"), []byte(agent), 0644); err != nil {
		t.Fatal(err)
	}
	r := Runner{Config: config.Config{ResearchRunner: "local", ResearchModel: model, ResearchMaxSteps: 8}}
	if err = os.WriteFile(filepath.Join(input, "opencode.json"), r.modelConfig(), 0644); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	r.tool, err = startCheckServer(func(ctx context.Context, res Result) []string {
		if calls.Add(1) == 1 {
			return []string{"explanation must mention the word banana"}
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.tool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd, cleanup, err := r.command(ctx, input, "live")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	cmd.Args[len(cmd.Args)-1] = `Call the tracker_check_result tool with result {"outcome":"not_relevant","explanation":"A test result that is not about cannabis enforcement at all."}. If it reports problems, fix them and call it again. Then reply with only the word done.`
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if calls.Load() < 2 {
		t.Fatalf("tool called %d times\n%s", calls.Load(), tail(string(out)))
	}
	t.Log(tail(string(out)))
}

func tail(s string) string {
	if len(s) > 3000 {
		return s[len(s)-3000:]
	}
	return strings.TrimSpace(s)
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
