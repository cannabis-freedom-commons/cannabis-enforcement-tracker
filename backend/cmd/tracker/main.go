package main

import (
	"context"
	"errors"
	n "fiatjaf.com/nostr"
	"flag"
	"fmt"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/api"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/harvest"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/nostr"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/worker"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"sync"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("service stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	checkAI := flag.Bool("check-ai", false, "Check OpenCode binary and run a short model test, then exit")
	keygen := flag.String("keygen", "", "Write new Nostr service keys to a new private env file, then exit")
	resolve := flag.String("resolve", "", "Resolve a submission after human review, then exit")
	outcome := flag.String("outcome", "", "Human resolution: duplicate, not_relevant, routed_to_cfi, insufficient_evidence or needs_review")
	explanation := flag.String("explanation", "", "Public explanation for a human resolution")
	reportSlug := flag.String("report-slug", "", "Existing report slug for a duplicate resolution")
	backup := flag.String("backup", "", "Write an online SQLite backup to a new file, then exit")
	retry := flag.String("retry", "", "Retry dead jobs/outbox for a submission ID, then exit")
	pruneJobs := flag.Bool("prune-jobs", false, "Clean disposable job workspaces and expired job logs, preserving sources and active jobs, then exit")
	flag.Parse()
	if *keygen != "" {
		f, err := os.OpenFile(*keygen, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintf(f, "NOSTR_GATEWAY_SECRET=%s\nNOSTR_BOT_SECRET=%s\n", n.Generate().Hex(), n.Generate().Hex())
		return err
	}
	var logLevel slog.Level
	if raw := os.Getenv("LOG_LEVEL"); raw != "" {
		if err := logLevel.UnmarshalText([]byte(raw)); err != nil {
			return fmt.Errorf("invalid LOG_LEVEL: %w", err)
		}
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))
	c, err := config.Load()
	if err != nil {
		return err
	}
	if *checkAI {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		health := research.NewHealth()
		(research.Runner{Config: c}).Check(ctx, health)
		snapshot := health.Snapshot()
		logStartupCheck(snapshot)
		if snapshot.Status != "ready" {
			return errors.New(snapshot.Message)
		}
		return nil
	}

	if *pruneJobs {
		s, err := store.Open(c.Database)
		if err != nil {
			return err
		}
		defer s.DB.Close()
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return (&worker.Worker{Store: s, Config: c}).CleanWorkspaces(ctx)
	}

	// Repair commands also take this lock: they must not mutate leases under a live worker.
	workerLock, err := store.LockWorker(c.Database)
	if err != nil {
		return err
	}
	defer workerLock.Close()
	s, err := store.Open(c.Database)
	if err != nil {
		return err
	}
	defer s.DB.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *resolve != "" {
		switch *outcome {
		case "duplicate", "not_relevant", "routed_to_cfi", "insufficient_evidence", "needs_review":
		default:
			return errors.New("invalid human outcome")
		}
		if len(*explanation) < 10 || len(*explanation) > 1500 {
			return errors.New("provide a short public explanation")
		}
		if *outcome == "duplicate" {
			if !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(*reportSlug) {
				return errors.New("existing report slug required")
			}
			reports, err := worker.Coverage(c.RepoPath)
			if err != nil {
				return err
			}
			found := false
			for _, r := range reports {
				if r.Slug == *reportSlug {
					found = true
				}
			}
			if !found {
				return errors.New("report does not exist")
			}
		}
		v, err := s.Get(ctx, *resolve)
		if err != nil {
			return err
		}
		if err = s.Transition(ctx, v.CaseID, *outcome, *explanation, *reportSlug, ""); err != nil {
			return err
		}
		_, err = s.DB.ExecContext(ctx, "UPDATE jobs SET status='done',lease_until=0 WHERE ref=? AND status IN ('queued','dead')", v.CaseID)
		return err
	}
	if *backup != "" {
		return s.Backup(ctx, *backup)
	}
	if *retry != "" {
		return s.Restart(ctx, *retry)
	}

	if err := s.Recover(ctx); err != nil {
		return fmt.Errorf("recover interrupted processing: %w", err)
	}
	if c.DailyCents == 0 {
		if err := s.ResumeBudgetWait(ctx); err != nil {
			return fmt.Errorf("resume unlimited research: %w", err)
		}
	}
	slog.Info("durable queue recovery complete")
	// runs recorded before reasoning tokens were counted: count them again from their logs
	if err := research.BackfillTokens(ctx, s); err != nil {
		slog.Warn("token backfill failed", "error", err)
	}
	health := research.NewHealth()
	w := worker.Worker{Store: s, Config: c, Health: health}
	checkDone := make(chan struct{})
	if !c.Demo && c.ResearchEnabled() {
		go func() {
			defer close(checkDone)
			(research.Runner{Config: c, Store: s}).Check(ctx, health)
			snapshot := health.Snapshot()
			logStartupCheck(snapshot)
		}()
	} else {
		health.Set(research.HealthSnapshot{Status: "unconfigured", Message: "OpenCode research is not configured."})
		close(checkDone)
	}
	if c.BotSecret != "" {
		w.Nostr, err = nostr.New(s, c)
		if err != nil {
			return err
		}
		slog.Info("Nostr enabled", "npub", w.Nostr.Npub())
	}
	collectorDone := make(chan struct{})
	if len(c.RSSFeeds)+len(c.GDELTQueries) > 0 || c.GoogleNewsEnabled || c.BingNewsEnabled {
		go func() {
			defer close(collectorDone)
			var collectors sync.WaitGroup
			if len(c.RSSFeeds)+len(c.GDELTQueries) > 0 {
				collectors.Add(1)
				go func() {
					defer collectors.Done()
					(harvest.Collector{Store: s, Feeds: c.RSSFeeds, Queries: c.GDELTQueries}).Run(ctx)
				}()
			}
			if c.GoogleNewsEnabled {
				queries := c.GoogleNewsQueries
				if len(queries) == 0 {
					queries = harvest.DefaultGoogleNewsQueries
				}
				collectors.Add(1)
				go func() {
					defer collectors.Done()
					(harvest.GoogleNews{Store: s, Queries: queries, Interval: c.GoogleNewsInterval, MaxArticles: c.GoogleNewsMaxArticles, QueriesPerScan: c.GoogleNewsQueriesPerScan}).Run(ctx)
				}()
			}
			if c.BingNewsEnabled {
				queries := c.BingNewsQueries
				if len(queries) == 0 {
					queries = harvest.DefaultBingNewsQueries
				}
				collectors.Add(1)
				go func() {
					defer collectors.Done()
					(harvest.BingNews{Store: s, Queries: queries, Interval: c.BingNewsInterval, MaxArticles: c.BingNewsMaxArticles, QueriesPerScan: c.BingNewsQueriesPerScan}).Run(ctx)
				}()
			}
			collectors.Wait()
		}()
	} else {
		close(collectorDone)
	}

	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	server := &http.Server{BaseContext: func(net.Listener) context.Context { return ctx }, Addr: c.Addr, Handler: (&api.Server{Store: s, Config: c, Health: health}).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = server.Shutdown(shutdown)
	}()
	slog.Info("intake listening", "address", c.Addr, "demo", c.Demo, "research_enabled", c.ResearchEnabled())
	err = server.ListenAndServe()
	cancel()
	<-done
	<-collectorDone
	<-checkDone
	<-shutdownDone
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Startup prompts/output are operator diagnostics, never public API fields.
func logStartupCheck(snapshot research.HealthSnapshot) {
	fields := []any{"status", snapshot.Status, "message", snapshot.Message, "version", snapshot.Version}
	if d := snapshot.Diagnostics; d != nil {
		fields = append(fields, "model", d.Model, "prompt", d.Prompt, "agent_instruction", d.AgentInstruction, "arguments", d.Arguments, "response", logExcerpt(d.Response, 4000), "exit_code", d.ExitCode, "duration_ms", d.DurationMS, "attempts", len(d.Attempts), "diagnostics_file", d.File)
		if d.Stderr != "" {
			fields = append(fields, "stderr", logExcerpt(d.Stderr, 4000))
		}
		if snapshot.Status == "failed" {
			fields = append(fields, "stdout", logExcerpt(d.Stdout, 16000))
		}
	}
	slog.Info("OpenCode startup check", fields...)
}
func logExcerpt(value string, limit int) string {
	text := []rune(value)
	if len(text) > limit {
		return string(text[:limit]) + "… [truncated; full output in diagnostics file]"
	}
	return value
}
