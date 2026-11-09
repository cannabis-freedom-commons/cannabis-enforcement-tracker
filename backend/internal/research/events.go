package research

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"math"
	"path/filepath"
	"regexp"
	"strings"
)

type executionEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionID"`
	Error     struct {
		Name string `json:"name"` // older OpenCode: name and data.message
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
		Type     string `json:"type"` // newer: type, message, the provider's HTTP status and answer
		Message  string `json:"message"`
		Status   int    `json:"status"`
		Response struct {
			Body string `json:"body"`
		} `json:"response"`
	} `json:"error"`
	Part struct {
		SessionID string `json:"sessionID"`
		Text      string `json:"text"`
		Tool      string `json:"tool"`
		State     struct {
			Status string          `json:"status"`
			Error  string          `json:"error"`
			Input  json.RawMessage `json:"input"`
			Output json.RawMessage `json:"output"`
			Title  string          `json:"title"`
		} `json:"state"`
		Cost   float64 `json:"cost"`
		Tokens struct {
			Input     int64 `json:"input"`
			Output    int64 `json:"output"`
			Reasoning int64 `json:"reasoning"` // billed as output
			Cache     struct {
				Read  int64 `json:"read"`
				Write int64 `json:"write"`
			} `json:"cache"`
		} `json:"tokens"`
	} `json:"part"`
}
type eventWriter struct {
	ctx              context.Context
	store            RunLog
	cid, run, secret string
	dir              string // the job's workspace: its host path never reaches the public chat
	buffer           []byte
	events           int
	steps            int // model steps finished, for the step dividers in the public chat
	failed           int // chat messages that could not be stored
}

func (w *eventWriter) Write(p []byte) (int, error) {
	w.buffer = append(w.buffer, p...)
	if len(w.buffer) > 4*1024*1024 {
		return 0, fmt.Errorf("event size exceeded")
	}
	for {
		index := bytes.IndexByte(w.buffer, '\n')
		if index < 0 {
			break
		}
		line := append([]byte(nil), w.buffer[:index]...)
		w.buffer = w.buffer[index+1:]
		if len(bytes.TrimSpace(line)) > 0 {
			if err := w.handle(line); err != nil {
				return 0, err
			}
		}
	}
	return len(p), nil
}

var sessionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,120}$`)
var secretPattern = regexp.MustCompile(`(?i)(bearer\s+\S+|\b(?:sk|oc|opencode)[-_][A-Za-z0-9_-]{16,})`)

// what a tool did, in words for the public chat, followed by what it was asked (all public data)
var toolNames = map[string]string{
	"read": "Reading research context", "glob": "Locating research files", "grep": "Checking existing report coverage",
	"list": "Listing research files", "skill": "Loading editorial instructions", "webfetch": "Opening a public source",
	"websearch": "Searching for corroborating coverage", "write": "Writing the draft report", "edit": "Revising the draft report",
	"patch": "Revising the draft report", "todowrite": "Planning the next steps", "todoread": "Reviewing the plan",
	"task": "Asking a helper agent", "bash": "Running a check",
	"tracker_find_reports": "Searching existing reports", "tracker_get_report": "Reading existing report",
	"tracker_check_result": "Checking the draft result against the service's rules",
}
var toolPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

// a tool error, on one line, credentials redacted
func (w *eventWriter) reason(text string) string {
	return w.safeN(strings.Join(strings.Fields(text), " "), 500)
}

func thousands(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func (w *eventWriter) safe(text string) string { return w.safeN(text, 1000) }

// the text with credentials and host paths redacted, cut to max characters; everything else is public data
func (w *eventWriter) safeN(text string, max int) string {
	if w.secret != "" {
		text = strings.ReplaceAll(text, w.secret, "[redacted]")
	}
	text = w.hidePaths(secretPattern.ReplaceAllString(text, "[redacted]"))
	r := []rune(text)
	if len(r) > max {
		text = string(r[:max]) + "…"
	}
	return text
}

// what a tool was asked to do: the address, the query, the file or the pattern
func (w *eventWriter) toolInput(tool string, raw json.RawMessage) string {
	var in map[string]any
	if json.Unmarshal(raw, &in) != nil {
		return ""
	}
	str := func(key string) string {
		v, _ := in[key].(string)
		return strings.Join(strings.Fields(v), " ")
	}
	switch tool {
	case "tracker_check_result":
		return "" // the whole draft: shown as the final answer, not here
	case "tracker_get_report":
		return w.safeN(str("slug"), 200)
	case "tracker_find_reports":
		var parts []string
		for _, key := range []string{"country", "date", "title", "place", "keywords"} {
			if v := str(key); v != "" {
				parts = append(parts, v)
			}
		}
		if names, ok := in["case_ids"].([]any); ok && len(names) > 0 {
			parts = append(parts, fmt.Sprintf("%d case numbers", len(names)))
		}
		if days, ok := in["window_days"].(float64); ok && days > 0 {
			parts = append(parts, fmt.Sprintf("within %d days", int(days)))
		}
		return w.safeN(w.hidePaths(strings.Join(parts, " · ")), 300)
	case "grep", "glob":
		if v := str("pattern"); v != "" {
			if inc := str("include"); inc != "" {
				v += " in " + inc
			}
			if where := w.hidePaths(str("path")); where != "" && where != "." {
				v += " (" + where + ")"
			}
			return w.safeN(w.hidePaths(v), 300)
		}
	}
	for _, key := range []string{"url", "query", "filePath", "file_path", "path", "pattern", "name", "description"} {
		if v := str(key); v != "" {
			return w.safeN(w.hidePaths(v), 300)
		}
	}
	return ""
}

// absolute host paths, shown relative to the workspace (or by their last part outside it)
var hostPath = regexp.MustCompile(`(^|[\s'"(=,\[])(?:/(?:Users|home|private|var|tmp|opt|Volumes|root|srv|mnt|workspace|app|data))(?:/[^\s'"/,;)]+)+/?`)

func (w *eventWriter) hidePaths(text string) string {
	if w.dir != "" {
		for _, root := range []string{filepath.Join(w.dir, ".research-input"), w.dir} {
			roots := []string{root}
			if real, err := filepath.EvalSymlinks(root); err == nil && real != root {
				roots = append(roots, real)
			}
			for _, r := range roots {
				text = strings.ReplaceAll(text, r+"/", "")
				text = strings.ReplaceAll(text, r, ".")
			}
		}
	}
	return hostPath.ReplaceAllStringFunc(text, func(p string) string {
		lead := ""
		if p != "" && p[0] != '/' {
			lead, p = p[:1], p[1:]
		}
		return lead + shortPath(p)
	})
}

func shortPath(p string) string {
	{
		if i := strings.Index(p, "/.research-input/"); i >= 0 {
			return p[i+len("/.research-input/"):]
		}
		return "…/" + filepath.Base(strings.TrimSuffix(p, "/"))
	}
}

// what a tracker tool answered, in a few public words (the reports are public; the draft is not shown)
func toolOutcome(tool string, raw json.RawMessage) string {
	var out string
	if json.Unmarshal(raw, &out) != nil || out == "" {
		return ""
	}
	switch tool {
	case "tracker_find_reports":
		compare, other, inOther := []string{}, []string{}, false
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Other possible matches") {
				inOther = true
			}
			if slug, _, ok := strings.Cut(strings.TrimPrefix(line, "- "), " | "); ok && strings.HasPrefix(line, "- ") {
				if inOther {
					other = append(other, slug)
				} else {
					compare = append(compare, slug)
				}
			}
		}
		if len(compare)+len(other) == 0 {
			return "no matching report"
		}
		var parts []string
		if len(compare) > 0 {
			parts = append(parts, "must compare "+slugList(compare))
		}
		if len(other) > 0 {
			parts = append(parts, fmt.Sprintf("%d other possible matches, best %s", len(other), slugList(other[:min(2, len(other))])))
		}
		return strings.Join(parts, "; ")
	case "tracker_get_report":
		if strings.HasPrefix(out, "No report has the slug") {
			return "no such report"
		}
		for _, line := range strings.Split(out, "\n") {
			if title, ok := strings.CutPrefix(line, "title: "); ok {
				return "“" + strings.Trim(strings.TrimSpace(title), `"'`) + "”"
			}
		}
	case "tracker_check_result":
		switch {
		case strings.HasPrefix(out, "No problems found"):
			return "no problems found"
		case strings.HasPrefix(out, "The service would reject"):
			n := strings.Count(out, "\n- ")
			if n == 1 {
				return "1 problem to fix"
			}
			return fmt.Sprintf("%d problems to fix", n)
		default:
			return "the draft could not be read"
		}
	}
	return ""
}

func slugList(slugs []string) string {
	if len(slugs) > 4 {
		return strings.Join(slugs[:4], ", ") + fmt.Sprintf(" and %d more", len(slugs)-4)
	}
	return strings.Join(slugs, ", ")
}

// what an OpenCode error says: who failed, the provider's HTTP status, its message and support code
var providerErrors = map[string]string{
	"provider.auth":            "the model provider refused the credentials",
	"provider.invalid-request": "the model provider rejected the request",
	"provider.internal":        "the model provider had an internal error",
	"provider.unknown":         "the model provider failed",
	"provider.rate-limit":      "the model provider's rate limit was reached",
}

func (w *eventWriter) errorDetail(e executionEvent) string {
	who := providerErrors[e.Error.Type]
	if who == "" {
		who = e.Error.Type
		if who == "" {
			who = e.Error.Name
		}
		if !toolPattern.MatchString(strings.NewReplacer(".", "_", "-", "_").Replace(strings.ToLower(who))) {
			who = ""
		}
	}
	why := strings.TrimSpace(e.Error.Message)
	if why == "" {
		why = strings.TrimSpace(e.Error.Data.Message)
	}
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(e.Error.Response.Body), &body)
	var b strings.Builder
	if who != "" {
		b.WriteString(": " + who)
	}
	if e.Error.Status >= 100 && e.Error.Status < 600 {
		fmt.Fprintf(&b, " (HTTP %d)", e.Error.Status)
	}
	if why = w.reason(why); why != "" {
		if b.Len() == 0 {
			b.WriteString(": ")
		} else {
			b.WriteString(" — ")
		}
		b.WriteString(strings.TrimRight(why, "."))
	}
	if code := strings.TrimSpace(body.Code); code != "" && toolPattern.MatchString(strings.ToLower(strings.ReplaceAll(code, "-", "_"))) {
		b.WriteString(". Provider code " + code)
	}
	return b.String()
}

func (w *eventWriter) handle(line []byte) error {
	var e executionEvent
	if json.Unmarshal(line, &e) != nil {
		return nil
	}
	session := e.SessionID
	if session == "" {
		session = e.Part.SessionID
	}
	if sessionPattern.MatchString(session) {
		if err := w.store.UpdateRun(w.ctx, w.run, store.RunUpdate{SessionID: &session}); err != nil {
			return err
		}
	}
	stepDone := ""
	if e.Type == "step_finish" {
		if e.Part.Tokens.Input >= 0 && e.Part.Tokens.Output >= 0 && e.Part.Tokens.Input < 20000000 && e.Part.Tokens.Output < 20000000 && !math.IsNaN(e.Part.Cost) && e.Part.Cost >= 0 && e.Part.Cost < 1000 {
			t := e.Part.Tokens
			if err := w.store.UpdateRun(w.ctx, w.run, store.RunUpdate{Usage: &store.RunUsage{Input: t.Input, Output: t.Output, Reasoning: t.Reasoning, CacheRead: t.Cache.Read, CacheWrite: t.Cache.Write, Cost: e.Part.Cost}}); err != nil {
				return err
			}
			w.steps++
			stepDone = fmt.Sprintf("Step %d done · %s tokens read, %s written", w.steps, thousands(e.Part.Tokens.Input+e.Part.Tokens.Cache.Read), thousands(e.Part.Tokens.Output+e.Part.Tokens.Reasoning))
			if e.Part.Tokens.Reasoning > 0 {
				stepDone += fmt.Sprintf(" (%s of them reasoning)", thousands(e.Part.Tokens.Reasoning))
			}
		}
	}
	if w.events >= 500 { // a long run keeps its last steps out of the chat rather than flooding it
		return nil
	}
	role, kind, text := "", "", ""
	switch e.Type {
	case "step_finish":
		// a step's start says nothing; its end says how much the model read and wrote
		role, kind, text = "system", "step", stepDone
	case "tool_use":
		name, ok := toolNames[e.Part.Tool]
		if !ok && toolPattern.MatchString(e.Part.Tool) {
			name, ok = "Using the "+e.Part.Tool+" tool", true
		}
		if ok {
			role, kind, text = "tool", "tool", name
			if what := w.toolInput(e.Part.Tool, e.Part.State.Input); what != "" {
				text += ": " + what
			}
			switch e.Part.State.Status {
			case "completed":
				if found := toolOutcome(e.Part.Tool, e.Part.State.Output); found != "" {
					text += " — " + w.safeN(found, 400) + "."
				} else {
					text += " — done."
				}
			case "error":
				text += " — failed"
				if why := w.reason(w.hidePaths(e.Part.State.Error)); why != "" {
					text += ": " + why
				}
				text += "."
			default:
				text += "…"
			}
		}
	case "text":
		value := strings.TrimSpace(e.Part.Text)
		switch {
		case value == "":
		case strings.HasPrefix(value, "{") || strings.HasPrefix(value, "```"):
			// the structured final answer, shown folded in the chat
			role, kind, text = "assistant", "answer", w.safeN(value, 20000)
		default:
			role, kind, text = "assistant", "message", w.safe(value)
		}
	case "reasoning":
		if value := strings.TrimSpace(e.Part.Text); value != "" {
			role, kind, text = "assistant", "reasoning", w.safeN(value, 4000)
		}
	case "error":
		text = "OpenCode reported an error" + w.errorDetail(e)
		role, kind, text = "system", "error", text+"."
	}
	if text == "" {
		return nil
	}
	w.events++
	// the chat is a view of the run: a message that cannot be stored must not stop OpenCode
	if err := w.store.AddAIEvent(w.ctx, w.cid, w.run, role, kind, text); err != nil {
		w.failed++
	}
	return nil
}
