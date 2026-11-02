package config

import (
	"errors"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/harvest"
)

type Config struct {
	BingNewsEnabled                                                             bool
	BingNewsQueries                                                             []harvest.BingNewsQuery
	BingNewsInterval                                                            time.Duration
	BingNewsMaxArticles, BingNewsQueriesPerScan                                 int
	GoogleNewsEnabled                                                           bool
	GoogleNewsQueries                                                           []harvest.GoogleNewsQuery
	GoogleNewsQueriesPerScan                                                    int
	GoogleNewsInterval                                                          time.Duration
	GoogleNewsMaxArticles                                                       int
	Addr, Database, PublicURL, RepoPath, Workspaces, IngestToken                string
	Origins, Relays, RSSFeeds, GDELTQueries, TrustedProxies                     []string
	GatewaySecret, BotSecret                                                    string
	ResearchImage, ResearchNetwork, ResearchModel, ResearchKey                  string
	ResearchRunner, OpenCodeBinary, OpenCodeAuthFile, OpenCodeConfigFile        string
	AutoPublish                                                                 bool
	AutoPush                                                                    bool // push the report repository after each report commit
	WorkerConcurrency                                                           int  // requests worked on at the same time
	SourceBrowser                                                               bool
	DevControls                                                                 bool
	RawAILogs                                                                   bool
	ResearchTimeout                                                             time.Duration
	JobLogRetention                                                             time.Duration
	ResearchMaxSteps                                                            int     // model steps a research run may take before it must answer
	ResearchWebSearch                                                           string  // the search service OpenCode 2 uses: exa, parallel, firecrawl, tavily, tinyfish or random
	PlanUSD, PlanAllowanceUSD                                                   float64 // a subscription: its monthly price and the model usage it includes, at API prices
	RunCents, DailyCents                                                        int
	GitHubAppID, GitHubInstallationID, GitHubKeyFile, GitHubRepo, WebhookSecret string
	Demo                                                                        bool
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func Load() (Config, error) {
	c := Config{Addr: env("HTTP_ADDR", "127.0.0.1:8080"), Database: env("DATABASE_PATH", "data/tracker.sqlite"), PublicURL: strings.TrimRight(env("PUBLIC_BASE_URL", "https://tracker.cannabisfreedom.fyi"), "/"), RepoPath: env("REPO_PATH", ".."), Workspaces: env("WORKSPACE_PATH", "data/jobs"), IngestToken: os.Getenv("INGEST_TOKEN"), Origins: strings.Split(env("CORS_ORIGINS", "https://tracker.cannabisfreedom.fyi"), ","), GatewaySecret: os.Getenv("NOSTR_GATEWAY_SECRET"), BotSecret: os.Getenv("NOSTR_BOT_SECRET"), ResearchImage: os.Getenv("RESEARCH_IMAGE"), ResearchNetwork: os.Getenv("RESEARCH_NETWORK"), ResearchModel: os.Getenv("RESEARCH_MODEL"), ResearchKey: os.Getenv("RESEARCH_API_KEY"), GitHubAppID: os.Getenv("GITHUB_APP_ID"), GitHubInstallationID: os.Getenv("GITHUB_INSTALLATION_ID"), GitHubKeyFile: os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE"), GitHubRepo: env("GITHUB_REPO", "burningtree/cannabis-freedom-index-tracker"), WebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"), Demo: env("DEMO_WORKER", "false") == "true"}
	var discoveryErr error
	c.GoogleNewsEnabled, discoveryErr = strconv.ParseBool(env("HARVEST_GOOGLE_NEWS_ENABLED", "true"))
	if discoveryErr != nil {
		return c, errors.New("invalid HARVEST_GOOGLE_NEWS_ENABLED")
	}
	c.GoogleNewsInterval, discoveryErr = time.ParseDuration(env("HARVEST_GOOGLE_NEWS_INTERVAL", "15m"))
	if discoveryErr != nil || c.GoogleNewsInterval < time.Minute || c.GoogleNewsInterval > 24*time.Hour {
		return c, errors.New("HARVEST_GOOGLE_NEWS_INTERVAL must be between 1m and 24h")
	}
	c.GoogleNewsMaxArticles, discoveryErr = strconv.Atoi(env("HARVEST_GOOGLE_NEWS_MAX_ARTICLES", "25"))
	if discoveryErr != nil || c.GoogleNewsMaxArticles < 1 || c.GoogleNewsMaxArticles > 100 {
		return c, errors.New("HARVEST_GOOGLE_NEWS_MAX_ARTICLES must be 1 to 100")
	}
	c.GoogleNewsQueriesPerScan, discoveryErr = strconv.Atoi(env("HARVEST_GOOGLE_NEWS_QUERIES_PER_SCAN", "12"))
	if discoveryErr != nil || c.GoogleNewsQueriesPerScan < 1 || c.GoogleNewsQueriesPerScan > 50 {
		return c, errors.New("HARVEST_GOOGLE_NEWS_QUERIES_PER_SCAN must be 1 to 50")
	}
	for _, raw := range strings.Split(os.Getenv("HARVEST_GOOGLE_NEWS_QUERIES"), "|") {
		if raw = strings.TrimSpace(raw); raw != "" {
			if len(raw) > 1000 {
				return c, errors.New("Google News query exceeds 1000 characters")
			}
			query, err := harvest.ParseGoogleNewsQuery(raw)
			if err != nil {
				return c, err
			}
			c.GoogleNewsQueries = append(c.GoogleNewsQueries, query)
		}
	}
	if len(c.GoogleNewsQueries) > 50 {
		return c, errors.New("at most 50 Google News queries")
	}
	c.BingNewsEnabled, discoveryErr = strconv.ParseBool(env("HARVEST_BING_NEWS_ENABLED", "true"))
	if discoveryErr != nil {
		return c, errors.New("invalid HARVEST_BING_NEWS_ENABLED")
	}
	c.BingNewsInterval, discoveryErr = time.ParseDuration(env("HARVEST_BING_NEWS_INTERVAL", "15m"))
	if discoveryErr != nil || c.BingNewsInterval < time.Minute || c.BingNewsInterval > 24*time.Hour {
		return c, errors.New("HARVEST_BING_NEWS_INTERVAL must be between 1m and 24h")
	}
	c.BingNewsMaxArticles, discoveryErr = strconv.Atoi(env("HARVEST_BING_NEWS_MAX_ARTICLES", "10"))
	if discoveryErr != nil || c.BingNewsMaxArticles < 1 || c.BingNewsMaxArticles > 100 {
		return c, errors.New("HARVEST_BING_NEWS_MAX_ARTICLES must be 1 to 100")
	}
	c.BingNewsQueriesPerScan, discoveryErr = strconv.Atoi(env("HARVEST_BING_NEWS_QUERIES_PER_SCAN", "4"))
	if discoveryErr != nil || c.BingNewsQueriesPerScan < 1 || c.BingNewsQueriesPerScan > 50 {
		return c, errors.New("HARVEST_BING_NEWS_QUERIES_PER_SCAN must be 1 to 50")
	}
	for _, raw := range strings.Split(os.Getenv("HARVEST_BING_NEWS_QUERIES"), "|") {
		if raw = strings.TrimSpace(raw); raw != "" {
			if len(raw) > 1000 {
				return c, errors.New("Bing News query exceeds 1000 characters")
			}
			query, err := harvest.ParseBingNewsQuery(raw)
			if err != nil {
				return c, err
			}
			c.BingNewsQueries = append(c.BingNewsQueries, query)
		}
	}
	if len(c.BingNewsQueries) > 50 {
		return c, errors.New("at most 50 Bing News queries")
	}
	c.SourceBrowser = env("SOURCE_BROWSER", "true") == "true"
	c.DevControls = env("DEV_CONTROLS", "false") == "true"
	c.RawAILogs = env("RAW_AI_LOGS", "false") == "true"
	c.JobLogRetention, discoveryErr = time.ParseDuration(env("JOB_LOG_RETENTION", "336h"))
	if discoveryErr != nil || c.JobLogRetention < time.Hour || c.JobLogRetention > 365*24*time.Hour {
		return c, errors.New("JOB_LOG_RETENTION must be between 1h and 8760h")
	}
	c.ResearchRunner = env("RESEARCH_RUNNER", "docker")
	c.OpenCodeBinary = env("OPENCODE_BIN", "opencode")
	c.OpenCodeAuthFile = os.Getenv("OPENCODE_AUTH_FILE")
	c.OpenCodeConfigFile = os.Getenv("OPENCODE_CONFIG")
	var boolErr error
	c.AutoPublish, boolErr = strconv.ParseBool(env("AUTO_PUBLISH", "true"))
	if boolErr != nil {
		return c, errors.New("invalid AUTO_PUBLISH")
	}
	var concurrencyErr error
	c.WorkerConcurrency, concurrencyErr = strconv.Atoi(env("WORKER_CONCURRENCY", "2"))
	if concurrencyErr != nil || c.WorkerConcurrency < 1 || c.WorkerConcurrency > 8 {
		return c, errors.New("WORKER_CONCURRENCY must be 1 to 8")
	}
	c.AutoPush, boolErr = strconv.ParseBool(env("AUTO_PUSH", "true"))
	if boolErr != nil {
		return c, errors.New("invalid AUTO_PUSH")
	}
	if c.ResearchRunner != "docker" && c.ResearchRunner != "local" {
		return c, errors.New("RESEARCH_RUNNER must be docker or local")
	}
	if raw := os.Getenv("TRUSTED_PROXY_CIDRS"); raw != "" {
		for _, v := range strings.Split(raw, ",") {
			if _, err := netip.ParsePrefix(v); err != nil {
				return c, errors.New("invalid trusted proxy CIDR")
			}
			c.TrustedProxies = append(c.TrustedProxies, v)
		}
	}
	if raw := os.Getenv("HARVEST_RSS_URLS"); raw != "" {
		c.RSSFeeds = strings.Split(raw, ",")
	}
	if raw := os.Getenv("HARVEST_GDELT_QUERIES"); raw != "" {
		c.GDELTQueries = strings.Split(raw, "|")
	}
	if len(c.RSSFeeds) > 10 || len(c.GDELTQueries) > 10 {
		return c, errors.New("at most 10 configured feeds and queries")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(c.GitHubRepo) {
		return c, errors.New("invalid GitHub repository")
	}
	var err error
	seconds, err := strconv.Atoi(env("RESEARCH_TIMEOUT_SECONDS", "900"))
	if err != nil || seconds < 10 || seconds > 1800 {
		return c, errors.New("invalid research timeout")
	}
	c.ResearchTimeout = time.Duration(seconds) * time.Second
	// with web search working, a run spends steps searching and opening sources; 12 was too few to
	// leave room for the answer
	c.ResearchMaxSteps, err = strconv.Atoi(env("RESEARCH_MAX_STEPS", "24"))
	if err != nil || c.ResearchMaxSteps < 4 || c.ResearchMaxSteps > 100 {
		return c, errors.New("invalid research step limit")
	}
	// a subscription such as OpenCode Go: what it costs a month and how much model usage, at API
	// prices, it includes; the request page shows a run's share of the subscription beside its API
	// price. 0 for either: no subscription, API prices only
	if c.PlanUSD, err = strconv.ParseFloat(env("RESEARCH_PLAN_USD", "0"), 64); err != nil || c.PlanUSD < 0 {
		return c, errors.New("invalid RESEARCH_PLAN_USD")
	}
	if c.PlanAllowanceUSD, err = strconv.ParseFloat(env("RESEARCH_PLAN_ALLOWANCE_USD", "0"), 64); err != nil || c.PlanAllowanceUSD < 0 {
		return c, errors.New("invalid RESEARCH_PLAN_ALLOWANCE_USD")
	}
	c.ResearchWebSearch = env("RESEARCH_WEBSEARCH", "exa")
	if !strings.Contains("|exa|parallel|firecrawl|tavily|tinyfish|random|", "|"+c.ResearchWebSearch+"|") {
		return c, errors.New("invalid web search provider (exa, parallel, firecrawl, tavily, tinyfish or random)")
	}
	c.RunCents, err = strconv.Atoi(env("RESEARCH_RUN_CENTS", "100"))
	if err != nil || c.RunCents < 1 {
		return c, errors.New("invalid run budget")
	}
	c.DailyCents, err = strconv.Atoi(env("RESEARCH_DAILY_CENTS", "1000"))
	if err != nil || c.DailyCents < 0 || (c.DailyCents > 0 && c.DailyCents < c.RunCents) {
		return c, errors.New("invalid daily budget")
	}
	for _, v := range append([]string{c.PublicURL}, c.Origins...) {
		u, e := url.Parse(v)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return c, errors.New("invalid public URL or CORS origin")
		}
	}
	if (c.GatewaySecret == "") != (c.BotSecret == "") {
		return c, errors.New("configure both Nostr identities")
	}
	if c.GatewaySecret != "" && c.GatewaySecret == c.BotSecret {
		return c, errors.New("Nostr identities must differ")
	}
	if raw := os.Getenv("NOSTR_RELAYS"); raw != "" {
		for _, v := range strings.Split(raw, ",") {
			u, e := url.Parse(v)
			if e != nil || u.Scheme != "wss" || u.Host == "" || u.User != nil {
				return c, errors.New("invalid relay")
			}
			c.Relays = append(c.Relays, v)
		}
	}
	seenRelays := map[string]bool{}
	for _, relay := range c.Relays {
		seenRelays[relay] = true
	}
	if len(c.Relays) > 10 {
		return c, errors.New("at most 10 relays")
	}
	if c.BotSecret != "" && len(seenRelays) < 2 {
		return c, errors.New("configure at least two relays")
	}
	if c.ResearchRunner == "docker" && c.ResearchImage != "" && (!regexp.MustCompile(`^[A-Za-z0-9._:/-]+@sha256:[a-f0-9]{64}$`).MatchString(c.ResearchImage) || c.ResearchNetwork == "" || c.ResearchModel == "" || c.ResearchKey == "") {
		return c, errors.New("research requires a digest-pinned image, restricted egress network, model and dedicated budget-limited API key")
	}
	if c.Demo && (c.BotSecret != "" || c.ResearchImage != "" || c.ResearchKey != "") {
		return c, errors.New("demo worker must run without Nostr or AI")
	}
	if c.IngestToken != "" && len(c.IngestToken) < 32 {
		return c, errors.New("ingest token must be at least 32 characters")
	}
	return c, nil
}

func (c Config) ResearchEnabled() bool {
	return c.ResearchModel != "" && (c.ResearchRunner == "local" || (c.ResearchImage != "" && c.ResearchKey != ""))
}
