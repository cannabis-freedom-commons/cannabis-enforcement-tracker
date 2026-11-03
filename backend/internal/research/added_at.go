package research

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Persist a legacy addition time before a supported edit can move/rewrite the file. This is an
// origin commit time, never the time of research, URL intake or a subsequent publication.
func firstReportAddition(dir, rel string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	git := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		return cmd.Output()
	}
	shallow, err := git("rev-parse", "--is-shallow-repository")
	if err != nil || strings.TrimSpace(string(shallow)) != "false" {
		return ""
	}
	output, err := git("log", "--first-parent", "--follow", "--diff-filter=A", "--format=%cI", "--", rel)
	if err != nil {
		return ""
	}
	value := strings.Split(strings.TrimSpace(string(output)), "\n")[0]
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return ""
	}
	return at.UTC().Format(time.RFC3339Nano)
}
