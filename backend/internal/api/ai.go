package api

import (
	"context"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type ProcessingInfo struct {
	Agents                []config.ResearchAgent   `json:"agents"`
	Budget                *BudgetInfo              `json:"budget"`
	Startup               *research.HealthSnapshot `json:"startup,omitempty"`
	RestartAvailable      bool                     `json:"restart_available"`
	PublicationMode       string                   `json:"publication_mode"`
	RawLogsAvailable      bool                     `json:"raw_logs_available"`
	Mode                  string                   `json:"mode"`
	Model                 string                   `json:"model"`
	Runner                string                   `json:"runner"`
	Enabled               bool                     `json:"enabled"`
	Reason                string                   `json:"reason"`
	AutomaticPublication  bool                     `json:"automatic_publication"`
	PublicationConfigured bool                     `json:"publication_configured"`
	Plan                  *Plan                    `json:"plan,omitempty"`
	MaxRunning            int                      `json:"max_running"` // total configured research worker capacity
}

// Plan is a subscription the runs are paid from: its monthly price and the usage it includes at
// API prices, so a run's real cost is its API price times PriceUSD/AllowanceUSD.
type Plan struct {
	PriceUSD     float64 `json:"price_usd"`
	AllowanceUSD float64 `json:"allowance_usd"`
}

func (s *Server) processingInfo(ctx context.Context) ProcessingInfo {
	c := s.Config
	p := ProcessingInfo{Budget: s.budgetInfo(ctx, time.Now()), RestartAvailable: s.localControls(), PublicationMode: "local_commit", RawLogsAvailable: c.RawAILogs, Mode: "automatic", Model: c.ResearchModel, Runner: "OpenCode (" + c.ResearchRunner + ")", Enabled: c.ResearchEnabled(), AutomaticPublication: true, PublicationConfigured: c.RepoPath != "", MaxRunning: c.MaxWorkers()}
	p.Agents = c.Agents()
	models := make([]string, 0, len(p.Agents))
	for _, a := range p.Agents {
		models = append(models, a.Model)
	}
	p.Model = strings.Join(models, ", ")
	if c.PlanUSD > 0 && c.PlanAllowanceUSD > 0 {
		p.Plan = &Plan{PriceUSD: c.PlanUSD, AllowanceUSD: c.PlanAllowanceUSD}
	}
	if c.Remote() && s.RemoteAgents != nil {
		agents, runner := s.RemoteAgents()
		p.Agents, p.Model, p.Runner, p.MaxRunning, p.Enabled = agents, "", "OpenCode on research workers", 0, len(agents) > 0
		if runner != "" {
			p.Runner = "OpenCode (" + runner + ", on research workers)"
		}
		models := []string{}
		for _, a := range agents {
			models = append(models, a.Model)
			p.MaxRunning += a.MaxWorkers
		}
		p.Model = strings.Join(models, ", ")
		if s.Health != nil {
			snapshot := s.Health.Snapshot()
			p.Startup = &snapshot
			p.Enabled = p.Enabled && snapshot.Status == "ready"
			p.Reason = snapshot.Message
		}
		return p
	}
	if c.Demo {
		p.Mode = "demo"
		p.Enabled = true
		p.Reason = "Demo worker simulates progress; no AI model is called."
		return p
	}
	if len(c.Agents()) == 0 {
		p.Reason = "AI processing is not configured. Set RESEARCH_AGENTS (or RESEARCH_MODEL) and configure OpenCode auth."
		p.Enabled = false
	} else if c.ResearchRunner != "local" && c.ResearchKey == "" {
		p.Reason = "Waiting for the AI API key. Add RESEARCH_API_KEY to .env and restart backend-dev."
		p.Enabled = false
	} else if c.ResearchRunner == "local" {
		binary := c.OpenCodeBinary
		if binary == "" {
			binary = "opencode"
		}
		if _, err := exec.LookPath(binary); err != nil {
			p.Enabled = false
			p.Reason = "The OpenCode CLI is unavailable."
		} else if runtime.GOOS != "darwin" {
			p.Enabled = false
			p.Reason = "The local filesystem sandbox requires macOS. Configure the Docker runner on this system."
		}
	} else if c.ResearchImage == "" {
		p.Enabled = false
		p.Reason = "The OpenCode Docker image and network are not configured."
	}
	if p.Enabled && p.Reason == "" {
		p.Reason = "OpenCode processes queued requests automatically."
	}
	if s.Health != nil && !c.Demo {
		snapshot := s.Health.Snapshot()
		p.Startup = &snapshot
		p.Enabled = p.Enabled && snapshot.Status == "ready"
		p.Reason = snapshot.Message
	}
	return p
}
func (s *Server) ai(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		problem(w, 404, "Receipt not found.")
		return
	}
	after := int64(-1)
	if raw := r.URL.Query().Get("after"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 {
			problem(w, 400, "Invalid AI activity cursor.")
			return
		}
		after = value
	}
	activity, err := s.Store.AIActivity(r.Context(), id, after)
	if err != nil {
		if store.IsMissingAI(err) {
			problem(w, 404, "Receipt not found.")
		} else {
			problem(w, 503, "AI activity unavailable.")
		}
		return
	}
	write(w, 200, struct {
		Processing ProcessingInfo `json:"processing"`
		Activity   any            `json:"activity"`
	}{s.processingInfo(r.Context()), activity})
}

func (s *Server) aiLog(w http.ResponseWriter, r *http.Request) {
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
			problem(w, 503, "AI log unavailable.")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, struct {
		SchemaVersion int              `json:"schema_version"`
		SubmissionID  string           `json:"submission_id"`
		Scope         string           `json:"scope"`
		Processing    ProcessingInfo   `json:"processing"`
		Activity      store.AIActivity `json:"activity"`
	}{1, id, "All recorded public AI activity; raw tool output and private reasoning are excluded.", s.processingInfo(r.Context()), activity})
}
