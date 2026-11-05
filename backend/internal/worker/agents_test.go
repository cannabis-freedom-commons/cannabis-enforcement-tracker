package worker

import (
	"sync"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func TestAgentPoolCapacityAndReadiness(t *testing.T) {
	c := config.Config{ResearchAgents: []config.ResearchAgent{{Model: "a/model#xhigh", MaxWorkers: 3}, {Model: "b/model#xhigh", MaxWorkers: 3}}}
	h := research.NewAgentHealth(c.Agents())
	p := newAgentPool(c, h)
	if _, _, ok := p.acquire(); ok {
		t.Fatal("checking agents allocated")
	}
	h.ForModel("a/model#xhigh").Set(research.HealthSnapshot{Status: "ready"})
	h.ForModel("b/model#xhigh").Set(research.HealthSnapshot{Status: "failed"})
	if !h.Ready() {
		t.Fatal("healthy agent blocked by failed peer")
	}
	var releases []func()
	for range 3 {
		model, release, ok := p.acquire()
		if !ok || model != "a/model#xhigh" {
			t.Fatal(model, ok)
		}
		releases = append(releases, release)
	}
	if _, _, ok := p.acquire(); ok {
		t.Fatal("exceeded per-agent limit")
	}
	h.ForModel("b/model#xhigh").Set(research.HealthSnapshot{Status: "ready"})
	for range 3 {
		model, release, ok := p.acquire()
		if !ok || model != "b/model#xhigh" {
			t.Fatal(model, ok)
		}
		releases = append(releases, release)
	}
	if _, _, ok := p.acquire(); ok {
		t.Fatal("exceeded total limit")
	}
	for _, release := range releases {
		release()
		release()
	}
	seen := map[string]bool{}
	for range 200 {
		model, release, ok := p.acquire()
		if !ok {
			t.Fatal("capacity not released")
		}
		seen[model] = true
		release()
	}
	if len(seen) != 2 {
		t.Fatal("random allocation never selected both agents")
	}
}

func TestAgentPoolRequiresPassingRecoveryProbeAfterLaunchFailures(t *testing.T) {
	c := config.Config{ResearchAgents: []config.ResearchAgent{{Model: "a/model", MaxWorkers: 1}}}
	p := newAgentPool(c, nil)
	now := time.Now()
	model, release, ok := p.acquire()
	if !ok {
		t.Fatal("initial reservation failed")
	}
	for range agentFailureLimit {
		p.observe(model, store.ErrResearchAgent, now)
	}
	later := now.Add(agentCooldown)
	if due := p.dueProbes(later); len(due) != 0 {
		t.Fatal("probe raced an active job")
	}
	release()
	if _, _, ok := p.acquire(); ok {
		t.Fatal("cooled agent took another job")
	}
	if due := p.dueProbes(later); len(due) != 1 {
		t.Fatal("expected one recovery probe", due)
	}
	if due := p.dueProbes(later); len(due) != 0 {
		t.Fatal("duplicate recovery probe", due)
	}
	p.finishProbe(0, false, later)
	if _, _, ok := p.acquire(); ok {
		t.Fatal("agent with failed probe took another job")
	}
	if due := p.dueProbes(later.Add(agentCooldown)); len(due) != 1 {
		t.Fatal("failed probe was not rescheduled", due)
	}
	p.finishProbe(0, true, later.Add(agentCooldown))
	if _, release, ok := p.acquire(); !ok {
		t.Fatal("recovered agent stayed paused")
	} else {
		release()
	}
}

func TestAgentPoolConcurrentReservations(t *testing.T) {
	c := config.Config{ResearchAgents: []config.ResearchAgent{{Model: "a/model", MaxWorkers: 3}, {Model: "b/model", MaxWorkers: 3}}}
	p := newAgentPool(c, nil)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_, release, ok := p.acquire()
				if ok {
					p.mu.Lock()
					for i, a := range p.agents {
						if p.running[i] > a.MaxWorkers {
							t.Error("capacity exceeded")
						}
					}
					p.mu.Unlock()
					release()
				}
			}
		}()
	}
	wg.Wait()
	for _, n := range p.running {
		if n != 0 {
			t.Fatal("leaked reservation", n)
		}
	}
}
