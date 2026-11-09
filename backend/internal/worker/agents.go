package worker

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

const agentFailureLimit = 3
const agentCooldown = time.Minute

type agentRecovery struct {
	failures int
	retryAt  time.Time
	probing  bool
}

// agentPool reserves capacity before claiming a durable job. Selection is uniform
// among ready agents with spare capacity, rather than weighted by worker count.
type agentPool struct {
	mu       sync.Mutex
	agents   []config.ResearchAgent
	running  []int
	recovery []agentRecovery
	health   *research.Health
}

func newAgentPool(c config.Config, health *research.Health) *agentPool {
	agents := c.Agents()
	return &agentPool{agents: agents, running: make([]int, len(agents)), recovery: make([]agentRecovery, len(agents)), health: health}
}
func (p *agentPool) acquire() (string, func(), bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	eligible := make([]int, 0, len(p.agents))
	for i, a := range p.agents {
		h := p.health.ForModel(a.Model)
		if p.running[i] < a.MaxWorkers && p.recovery[i].retryAt.IsZero() && (p.health == nil || h.Ready()) {
			eligible = append(eligible, i)
		}
	}
	if len(eligible) == 0 {
		return "", nil, false
	}
	i := eligible[rand.IntN(len(eligible))]
	p.running[i]++
	var once sync.Once
	return p.agents[i].Model, func() { once.Do(func() { p.mu.Lock(); p.running[i]--; p.mu.Unlock() }) }, true
}

// Record outcomes from actual OpenCode invocations. Fetching and verification
// errors are unrelated to the health of the selected model.
func (p *agentPool) observe(model string, err error, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, agent := range p.agents {
		if agent.Model != model {
			continue
		}
		state := &p.recovery[i]
		if !state.retryAt.IsZero() {
			return
		}
		if !errors.Is(err, store.ErrResearchAgent) {
			state.failures = 0
			return
		}
		state.failures++
		if state.failures >= agentFailureLimit {
			state.retryAt = now.Add(agentCooldown)
			if h := p.health.ForModel(model); h != nil {
				h.Set(research.HealthSnapshot{Status: "checking", Message: "Research agent paused after repeated launch failures. An automatic model check will run before it resumes.", CheckedAt: now.Unix()})
			}
			slog.Warn("research agent cooling down", "model", model, "retry_at", state.retryAt)
		}
		return
	}
}

func (p *agentPool) dueProbes(now time.Time) []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	var due []int
	for i := range p.recovery {
		state := &p.recovery[i]
		// Let older invocations report their outcomes before reopening the pool.
		if !state.retryAt.IsZero() && !now.Before(state.retryAt) && !state.probing && p.running[i] == 0 {
			state.probing = true
			due = append(due, i)
		}
	}
	return due
}

func (p *agentPool) finishProbe(i int, ready bool, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recovery[i] = agentRecovery{}
	if !ready {
		p.recovery[i].retryAt = now.Add(agentCooldown)
	}
}

func (p *agentPool) recoverAgents(ctx context.Context, c config.Config, s research.RunLog) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var probes sync.WaitGroup
	defer probes.Wait()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			for _, i := range p.dueProbes(now) {
				probes.Go(func() {
					selected := c
					selected.ResearchModel = p.agents[i].Model
					h := p.health.ForModel(selected.ResearchModel)
					if h == nil {
						h = research.NewHealth()
					}
					(research.Runner{Config: selected, Store: s}).Check(ctx, h)
					p.finishProbe(i, h.Ready(), time.Now())
				})
			}
		}
	}
}
