package config

import (
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/harvest"
)

func defaults(t *testing.T) {
	t.Helper()
	for _, k := range []string{"NOSTR_GATEWAY_SECRET", "NOSTR_BOT_SECRET", "NOSTR_RELAYS", "RESEARCH_IMAGE", "RESEARCH_NETWORK", "RESEARCH_MODEL", "RESEARCH_AGENTS", "WORKER_CONCURRENCY", "RESEARCH_API_KEY", "GITHUB_APP_ID", "GITHUB_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY_FILE", "INGEST_TOKEN", "TRUSTED_PROXY_CIDRS", "HARVEST_RSS_URLS", "HARVEST_GDELT_QUERIES", "HARVEST_BING_NEWS_ENABLED", "HARVEST_BING_NEWS_INTERVAL", "HARVEST_BING_NEWS_QUERIES", "HARVEST_BING_NEWS_MAX_ARTICLES", "HARVEST_BING_NEWS_QUERIES_PER_SCAN", "HARVEST_GOOGLE_NEWS_ENABLED", "HARVEST_GOOGLE_NEWS_INTERVAL", "HARVEST_GOOGLE_NEWS_QUERIES", "HARVEST_GOOGLE_NEWS_MAX_ARTICLES", "HARVEST_GOOGLE_NEWS_QUERIES_PER_SCAN"} {
		t.Setenv(k, "")
	}
	t.Setenv("DEMO_WORKER", "false")
}
func TestOptionalIntegrationsDisabledByDefault(t *testing.T) {
	defaults(t)
	c, err := Load()
	if err != nil || c.ResearchImage != "" || c.BotSecret != "" {
		t.Fatal(err, c)
	}
}
func TestInvalidConfigurationFailsClosed(t *testing.T) {
	for _, value := range []struct{ key, value string }{{"CORS_ORIGINS", "*"}, {"NOSTR_BOT_SECRET", strings.Repeat("a", 64)}, {"RESEARCH_IMAGE", "image:latest"}, {"GITHUB_REPO", "invalid"}, {"TRUSTED_PROXY_CIDRS", "anyone"}, {"INGEST_TOKEN", "short"}} {
		t.Run(value.key, func(t *testing.T) {
			defaults(t)
			t.Setenv(value.key, value.value)
			if _, err := Load(); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
}

func TestDailyBudgetZeroIsUnlimitedButOtherBoundsRemain(t *testing.T) {
	for _, test := range []struct {
		value string
		valid bool
	}{{"0", true}, {"100", true}, {"99", false}, {"-1", false}, {"invalid", false}} {
		t.Run(test.value, func(t *testing.T) {
			defaults(t)
			t.Setenv("RESEARCH_RUN_CENTS", "100")
			t.Setenv("RESEARCH_DAILY_CENTS", test.value)
			_, err := Load()
			if (err == nil) != test.valid {
				t.Fatal(test, err)
			}
		})
	}
}

func TestResearchModelComesFromEnvironment(t *testing.T) {
	for _, runner := range []string{"local", "docker"} {
		t.Run(runner, func(t *testing.T) {
			defaults(t)
			t.Setenv("RESEARCH_RUNNER", runner)
			c, err := Load()
			if err != nil || c.ResearchModel != "" || c.ResearchEnabled() {
				t.Fatalf("missing model should disable research: model=%q err=%v", c.ResearchModel, err)
			}
			t.Setenv("RESEARCH_MODEL", "other-provider/selected-model#high")
			c, err = Load()
			if err != nil || c.ResearchModel != "other-provider/selected-model#high" {
				t.Fatalf("configured model was not preserved: model=%q err=%v", c.ResearchModel, err)
			}
			if runner == "local" && !c.ResearchEnabled() {
				t.Fatal("configured local model should enable research")
			}
		})
	}
}

func TestGoogleNewsConfiguration(t *testing.T) {
	defaults(t)
	c, err := Load()
	if err != nil || !c.GoogleNewsEnabled || c.GoogleNewsInterval != 2*time.Minute || c.GoogleNewsMaxArticles != 15 || c.GoogleNewsQueriesPerScan != 8 {
		t.Fatal(c, err)
	}
	t.Setenv("HARVEST_GOOGLE_NEWS_ENABLED", "false")
	t.Setenv("HARVEST_GOOGLE_NEWS_QUERIES", " cannabis raid | [CZ:cs] konopí policie ")
	c, err = Load()
	if err != nil || c.GoogleNewsEnabled || len(c.GoogleNewsQueries) != 2 || c.GoogleNewsQueries[0] != (harvest.GoogleNewsQuery{Edition: "US:en", Query: "cannabis raid"}) || c.GoogleNewsQueries[1] != (harvest.GoogleNewsQuery{Edition: "CZ:cs", Query: "konopí policie"}) {
		t.Fatal(c, err)
	}
	for key, value := range map[string]string{"HARVEST_GOOGLE_NEWS_ENABLED": "invalid", "HARVEST_GOOGLE_NEWS_INTERVAL": "0s", "HARVEST_GOOGLE_NEWS_MAX_ARTICLES": "-1", "HARVEST_GOOGLE_NEWS_QUERIES_PER_SCAN": "0", "HARVEST_GOOGLE_NEWS_QUERIES": "[cz] konopí"} {
		t.Run(key, func(t *testing.T) {
			defaults(t)
			t.Setenv(key, value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid value accepted")
			}
		})
	}
}

func TestGoogleNewsExpandedQueryLimit(t *testing.T) {
	for _, count := range []int{50, harvest.MaxGoogleNewsQueries, harvest.MaxGoogleNewsQueries + 1} {
		defaults(t)
		t.Setenv("HARVEST_GOOGLE_NEWS_QUERIES", strings.Repeat("cannabis seized|", count))
		c, err := Load()
		if count <= harvest.MaxGoogleNewsQueries {
			if err != nil || len(c.GoogleNewsQueries) != count {
				t.Fatal("expanded rotation rejected", count, len(c.GoogleNewsQueries), err)
			}
		} else if err == nil {
			t.Fatal("unbounded query rotation accepted", count)
		}
	}
	defaults(t)
	t.Setenv("HARVEST_GOOGLE_NEWS_QUERIES_PER_SCAN", "51")
	if _, err := Load(); err == nil {
		t.Fatal("expanded rotation must retain the per-scan workload limit")
	}
}

func TestJobLogRetention(t *testing.T) {
	for _, value := range []struct {
		raw   string
		valid bool
	}{{"", true}, {"1h", true}, {"336h", true}, {"8760h", true}, {"0", false}, {"-1h", false}, {"30m", false}, {"8761h", false}, {"forever", false}} {
		t.Run(value.raw, func(t *testing.T) {
			defaults(t)
			t.Setenv("JOB_LOG_RETENTION", value.raw)
			c, err := Load()
			if (err == nil) != value.valid {
				t.Fatal(c.JobLogRetention, err)
			}
			if value.raw == "" && c.JobLogRetention != 14*24*time.Hour {
				t.Fatal("wrong default", c.JobLogRetention)
			}
		})
	}
}

func TestBingNewsConfiguration(t *testing.T) {
	defaults(t)
	c, err := Load()
	if err != nil || !c.BingNewsEnabled || c.BingNewsInterval != 3*time.Minute || c.BingNewsMaxArticles != 8 || c.BingNewsQueriesPerScan != 5 {
		t.Fatal(c, err)
	}
	t.Setenv("HARVEST_BING_NEWS_ENABLED", "false")
	t.Setenv("HARVEST_BING_NEWS_QUERIES", " cannabis raid | [zh-CN] 大麻 查获 ")
	c, err = Load()
	if err != nil || c.BingNewsEnabled || len(c.BingNewsQueries) != 2 || c.BingNewsQueries[0] != (harvest.BingNewsQuery{Market: "en-US", Query: "cannabis raid"}) || c.BingNewsQueries[1] != (harvest.BingNewsQuery{Market: "zh-HK", Query: "大麻 查获"}) {
		t.Fatal(c, err)
	}
	for key, values := range map[string][]string{
		"HARVEST_BING_NEWS_ENABLED":          {"invalid"},
		"HARVEST_BING_NEWS_INTERVAL":         {"0s", "25h", "bad"},
		"HARVEST_BING_NEWS_MAX_ARTICLES":     {"0", "101", "bad"},
		"HARVEST_BING_NEWS_QUERIES_PER_SCAN": {"0", "51", "bad"},
		"HARVEST_BING_NEWS_QUERIES":          {"[zh-cn] 大麻", strings.Repeat("a", 1001), strings.Repeat("cannabis|", harvest.MaxBingNewsQueries+1)},
	} {
		for _, value := range values {
			t.Run(key+"/"+value[:min(20, len(value))], func(t *testing.T) {
				defaults(t)
				t.Setenv(key, value)
				if _, err := Load(); err == nil {
					t.Fatal("invalid value accepted")
				}
			})
		}
	}
}

func TestResearchAgentsConfiguration(t *testing.T) {
	defaults(t)
	t.Setenv("RESEARCH_RUNNER", "local")
	t.Setenv("RESEARCH_MODEL", "legacy/model")
	t.Setenv("WORKER_CONCURRENCY", "invalid-but-overridden")
	t.Setenv("RESEARCH_AGENTS", `[{"model":"opencode-go/muse-spark-1.3-contributor#xhigh","max_workers":3},{"model":"swancloud/muse-spark-1.3-contributor#xhigh","max_workers":3}]`)
	c, err := Load()
	if err != nil || len(c.Agents()) != 2 || c.MaxWorkers() != 6 || !c.ResearchEnabled() {
		t.Fatal(c, err)
	}
	for _, raw := range []string{`[]`, `null`, `{}`, `[{}]`, `[{"model":"p/m","max_workers":0}]`, `[{"model":"p/m","max_workers":9}]`, `[{"model":"p/m","max_workers":1,"typo":2}]`, `[{"model":"p/m","max_workers":1},{"model":"p/m","max_workers":2}]`, `[{"model":"invalid","max_workers":1}]`, `[{"model":"p/m","max_workers":1}] []`} {
		t.Setenv("RESEARCH_AGENTS", raw)
		if _, err := Load(); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestHarvestQueueTarget(t *testing.T) {
	defaults(t)
	t.Setenv("WORKER_CONCURRENCY", "3")
	c, err := Load()
	if err != nil || c.HarvestQueueTarget != -1 || c.QueueTarget() != 4*c.MaxWorkers() {
		t.Fatal("automatic queue target", c.QueueTarget(), c.MaxWorkers(), err)
	}
	t.Setenv("HARVEST_QUEUE_TARGET", "0")
	if c, err = Load(); err != nil || c.QueueTarget() != 0 {
		t.Fatal("backpressure cannot be disabled", c.QueueTarget(), err)
	}
	t.Setenv("HARVEST_QUEUE_TARGET", "30")
	if c, err = Load(); err != nil || c.QueueTarget() != 30 {
		t.Fatal(c.QueueTarget(), err)
	}
	for _, bad := range []string{"-2", "1001", "many"} {
		t.Setenv("HARVEST_QUEUE_TARGET", bad)
		if _, err := Load(); err == nil {
			t.Fatal("invalid queue target accepted", bad)
		}
	}
}
