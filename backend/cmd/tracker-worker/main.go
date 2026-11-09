// tracker-worker researches submissions for a tracker backend (docs/WORKERS.md): it takes research
// jobs from the backend's worker API, opens the pages, runs the AI research and checks, and sends
// every result back. It needs no database and publishes nothing itself.
//
// Configuration comes from the environment (see backend/worker.example.env): TRACKER_URL,
// WORKER_TOKEN, REPO_PATH (the worker's own clone of the reports repository), WORKSPACE_PATH and
// the research settings (RESEARCH_AGENTS or RESEARCH_MODEL, RESEARCH_RUNNER, OpenCode files).
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	checkAI := flag.Bool("check-ai", false, "Check OpenCode and run a short model test, then exit")
	flag.Parse()
	var level slog.Level
	if raw := os.Getenv("LOG_LEVEL"); raw != "" {
		if err := level.UnmarshalText([]byte(raw)); err != nil {
			return errors.New("invalid LOG_LEVEL")
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	c, err := config.Load()
	if err != nil {
		return err
	}
	if !c.ResearchEnabled() {
		return errors.New("configure RESEARCH_AGENTS or RESEARCH_MODEL (and the OpenCode runner) for this worker")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	health := research.NewAgentHealth(c.Agents())
	check := func(ctx context.Context) {
		done := make(chan struct{})
		agents := c.Agents()
		for _, agent := range agents {
			go func() {
				defer func() { done <- struct{}{} }()
				selected := c
				selected.ResearchModel = agent.Model
				h := health.ForModel(agent.Model)
				(research.Runner{Config: selected}).Check(ctx, h)
				s := h.Snapshot()
				slog.Info("research agent checked", "model", agent.Model, "status", s.Status, "message", s.Message)
			}()
		}
		for range agents {
			<-done
		}
	}
	if *checkAI {
		check(ctx)
		if !health.Ready() {
			return errors.New(health.Snapshot().Message)
		}
		return nil
	}
	if c.TrackerURL == "" || !strings.HasPrefix(c.TrackerURL, "http") || len(c.WorkerToken) < 32 {
		return errors.New("set TRACKER_URL (the backend's address) and WORKER_TOKEN (one of its WORKER_TOKENS)")
	}
	if _, err := os.Stat(c.RepoPath + "/.git"); err != nil {
		return errors.New("REPO_PATH must be this worker's own clone of the reports repository")
	}
	go check(ctx)
	client := &worker.Client{URL: c.TrackerURL, Token: c.WorkerToken, HTTP: &http.Client{Timeout: 15 * time.Minute}}
	slog.Info("worker started", "backend", c.TrackerURL, "agents", len(c.Agents()), "slots", c.MaxWorkers(), "repo", c.RepoPath)
	(&worker.Worker{Config: c, Health: health}).RunRemote(ctx, client)
	slog.Info("worker stopped; its leased jobs return to the queue")
	return nil
}
