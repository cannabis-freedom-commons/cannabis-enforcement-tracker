package worker

import (
	"math/rand/v2"
	"sync"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
)

// agentPool reserves capacity before claiming a durable job. Selection is uniform
// among ready agents with spare capacity, rather than weighted by worker count.
type agentPool struct {
	mu      sync.Mutex
	agents  []config.ResearchAgent
	running []int
	health  *research.Health
}

func newAgentPool(c config.Config, health *research.Health) *agentPool {
	agents := c.Agents()
	return &agentPool{agents: agents, running: make([]int, len(agents)), health: health}
}
func (p *agentPool) acquire() (string, func(), bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	eligible := make([]int, 0, len(p.agents))
	for i, a := range p.agents {
		h := p.health.ForModel(a.Model)
		if p.running[i] < a.MaxWorkers && (p.health == nil || h.Ready()) {
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
