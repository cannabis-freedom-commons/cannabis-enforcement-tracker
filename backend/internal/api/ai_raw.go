package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/archivefile"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

type rawAIRun struct {
	RunID       string            `json:"run_id"`
	Available   bool              `json:"available"`
	Events      []json.RawMessage `json:"events"`
	PartialLine string            `json:"partial_line,omitempty"`
	Stderr      string            `json:"stderr"`
}

// Explicitly enabled diagnostics; the normal public transcript stays filtered.
func (s *Server) aiRaw(w http.ResponseWriter, r *http.Request) {
	if !s.Config.RawAILogs {
		problem(w, 404, "Raw AI logs are not enabled on this backend.")
		return
	}
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		problem(w, 404, "Receipt not found.")
		return
	}
	activity, err := s.Store.AIActivityLog(r.Context(), id)
	if err != nil {
		if store.IsMissingAI(err) {
			problem(w, 404, "Receipt not found.")
		} else {
			problem(w, 503, "Raw AI logs unavailable.")
		}
		return
	}
	runs := []rawAIRun{}
	for _, run := range activity.Runs {
		var workspace string
		// Fence every file lookup to a run belonging to this receipt's case.
		err = s.Store.DB.QueryRowContext(r.Context(), "SELECT workspace FROM research_runs WHERE id=? AND case_id=(SELECT case_id FROM submissions WHERE id=?)", run.ID, id).Scan(&workspace)
		if err != nil {
			problem(w, 503, "Raw AI logs unavailable.")
			return
		}
		entry := rawAIRun{RunID: run.ID, Events: []json.RawMessage{}}
		if workspace == "" {
			runs = append(runs, entry)
			continue
		}
		stdout, available, err := s.rawLogFile(workspace, "research-events.jsonl", 4*1024*1024)
		if err != nil {
			problem(w, 503, "Raw AI log could not be read.")
			return
		}
		entry.Available = available
		lines := bytes.Split(stdout, []byte("\n"))
		for i, line := range lines {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			if i == len(lines)-1 && !json.Valid(line) {
				entry.PartialLine = s.redactRawLog(string(line))
				continue
			}
			// Keep all event types, including reasoning, arguments and tool results.
			var value any
			decoder := json.NewDecoder(bytes.NewReader(line))
			decoder.UseNumber()
			if decoder.Decode(&value) != nil {
				value = map[string]any{"raw_line": string(line)}
			}
			value = s.redactRawValue(value)
			encoded, _ := json.Marshal(value)
			entry.Events = append(entry.Events, encoded)
		}
		stderr, _, err := s.rawLogFile(workspace, "research-stderr.log", 256*1024)
		if err != nil {
			problem(w, 503, "Raw AI diagnostics could not be read.")
			return
		}
		entry.Stderr = s.redactRawLog(string(stderr))
		runs = append(runs, entry)
	}
	write(w, 200, struct {
		SchemaVersion int              `json:"schema_version"`
		SubmissionID  string           `json:"submission_id"`
		Scope         string           `json:"scope"`
		Activity      store.AIActivity `json:"activity"`
		RawRuns       []rawAIRun       `json:"raw_runs"`
	}{1, id, "Raw OpenCode events, including emitted reasoning and tool output. Configured credentials are redacted; unexposed provider reasoning is unavailable.", activity, runs})
}

func (s *Server) rawLogFile(workspace, name string, limit int64) ([]byte, bool, error) {
	base, err := filepath.EvalSymlinks(s.Config.Workspaces)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return nil, false, err
	}
	dir, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, false, err
	}
	relative, err := filepath.Rel(base, dir)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return nil, false, errors.New("workspace outside job storage")
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, false, err
	}
	defer root.Close()
	file, err := archivefile.OpenRoot(root, filepath.Join(relative, name))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return nil, false, errors.New("raw log exceeds capture limit")
	}
	return data, true, nil
}

var rawSecretPattern = regexp.MustCompile(`(?i)(bearer\s+[^\s"'\\]+|(?:sk|oc|opencode)[-_][A-Za-z0-9_-]{16,})`)

func (s *Server) redactRawLog(value string) string {
	for _, secret := range []string{s.Config.ResearchKey, s.Config.IngestToken, s.Config.GatewaySecret, s.Config.BotSecret, s.Config.WebhookSecret} {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	return rawSecretPattern.ReplaceAllString(value, "[redacted]")
}
func (s *Server) redactRawValue(value any) any {
	switch v := value.(type) {
	case string:
		return s.redactRawLog(v)
	case []any:
		for i, item := range v {
			v[i] = s.redactRawValue(item)
		}
	case map[string]any:
		for key, item := range v {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			switch normalized {
			case "apikey", "authorization", "accesstoken", "refreshtoken", "password", "credential":
				v[key] = "[redacted]"
			default:
				v[key] = s.redactRawValue(item)
			}
		}
	}
	return value
}
