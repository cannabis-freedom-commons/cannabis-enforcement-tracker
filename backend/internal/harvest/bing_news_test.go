package harvest

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func TestBingPublisher(t *testing.T) {
	target := "https://publisher.example/story?edition=cn&utm_source=bing"
	wrapper := "http://www.bing.com/news/apiclick.aspx?tid=changing&url=" + url.QueryEscape(target)
	for _, raw := range []string{wrapper, target} {
		got, err := bingPublisher(raw)
		if err != nil || got != "https://publisher.example/story?edition=cn" {
			t.Fatal(got, err)
		}
	}
	for _, raw := range []string{
		"http://127.0.0.1/private", "https://www.bing.com/news/search?q=cannabis",
		"https://www.bing.com/news/apiclick.aspx", "https://www.bing.com/news/apiclick.aspx?url=" + url.QueryEscape("https://www.bing.com/news/apiclick.aspx?url=https://publisher.example/"),
		"https://www.bing.com/news/apiclick.aspx?url=" + url.QueryEscape("javascript:alert(1)"),
		"https://www.bing.com/news/apiclick.aspx?url=" + url.QueryEscape("http://192.168.1.1/private"),
		"https://www.bing.com/news/apiclick.aspx?url=" + url.QueryEscape("https://publisher.example:8443/"),
	} {
		if got, err := bingPublisher(raw); err == nil {
			t.Fatal("unsafe/unresolved destination accepted", raw, got)
		}
	}
}
func bingTestItem(link string, at time.Time) string {
	return "<item><link>" + html.EscapeString(link) + "</link><pubDate>" + at.UTC().Format(http.TimeFormat) + "</pubDate></item>"
}
func TestBingScanDeduplicatesPublishersAndSkipsOldOrInvalidResults(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bing.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	now := time.Now()
	wrap := func(raw string) string { return "https://www.bing.com/news/apiclick.aspx?url=" + url.QueryEscape(raw) }
	// Seed an existing Google discovery to check cross-provider deduplication.
	_, err = s.Accept(context.Background(), store.Intake{Origin: "rss", URL: "https://publisher.example/known", CanonicalURL: "https://publisher.example/known", IdempotencyKey: "google-news:known"})
	if err != nil {
		t.Fatal(err)
	}
	feed := `<rss><channel>` +
		bingTestItem(wrap("https://publisher.example/new?utm_source=bing"), now) +
		bingTestItem(wrap("https://publisher.example/new"), now) +
		bingTestItem(wrap("https://publisher.example/known"), now) +
		bingTestItem(wrap("https://publisher.example/old"), now.Add(-8*24*time.Hour)) +
		bingTestItem(wrap("https://publisher.example/future"), now.Add(48*time.Hour)) +
		`<item><link>https://publisher.example/undated</link></item>` +
		bingTestItem(wrap("http://127.0.0.1/private"), now) + `</channel></rss>`
	g := BingNews{Store: s, Queries: []BingNewsQuery{{"en-US", "cannabis seized"}}, client: &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "www.bing.com" || r.URL.Path != "/news/search" || r.URL.Query().Get("format") != "rss" || r.URL.Query().Get("mkt") != "en-US" || r.URL.Query().Get("q") != "cannabis seized" {
			t.Error("incorrect request", r.URL)
		}
		return newsResponse(r, feed), nil
	})}}
	stats, err := g.Scan(context.Background())
	if err != nil || stats.Found != 7 || stats.Queued != 1 || stats.Known != 2 || stats.Skipped != 3 || stats.Unresolved != 1 {
		t.Fatal(stats, err)
	}
	stats, err = g.Scan(context.Background())
	if err != nil || stats.Queued != 0 || stats.Known != 3 {
		t.Fatal(stats, err)
	}
	var jobs, posts int
	s.DB.QueryRow("SELECT count(*) FROM jobs WHERE kind='research'").Scan(&jobs)
	s.DB.QueryRow("SELECT count(*) FROM outbox").Scan(&posts)
	var provenance string
	s.DB.QueryRow("SELECT payload FROM jobs WHERE kind='research' AND ref=(SELECT id FROM cases WHERE canonical_url='https://publisher.example/new')").Scan(&provenance)
	var p map[string]any
	if err = json.Unmarshal([]byte(provenance), &p); err != nil {
		t.Fatal(err, provenance)
	}
	if jobs != 2 || posts != 0 || p["provider"] != "bing-news" || p["market"] != "en-US" || p["query"] != "cannabis seized" {
		t.Fatal(jobs, posts, p)
	}
}
func TestBingScanFairBudgetAndFailedMarket(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bing.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	g := BingNews{Store: s, MaxArticles: 2, Queries: []BingNewsQuery{{"zh-CN", "bad"}, {"en-US", "one"}, {"en-GB", "two"}}, client: &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("q")
		if q == "bad" {
			return newsResponse(r, "<html>verification</html>"), nil
		}
		return newsResponse(r, "<rss><channel>"+bingTestItem("https://publisher.example/"+q+"-1", time.Now())+bingTestItem("https://publisher.example/"+q+"-2", time.Now())+"</channel></rss>"), nil
	})}}
	stats, err := g.Scan(context.Background())
	if err != nil || stats.Queued != 2 || stats.Found != 4 {
		t.Fatal(stats, err)
	}
	var count int
	s.DB.QueryRow("SELECT count(*) FROM cases WHERE canonical_url IN ('https://publisher.example/one-1','https://publisher.example/two-1')").Scan(&count)
	if count != 2 {
		t.Fatal("one market exhausted budget", count)
	}
}
func TestBingQueriesAndFeeds(t *testing.T) {
	for _, raw := range []string{"[en-US]", "[CN] 大麻", "[zh-cn] 大麻", "[en-US cannabis", ""} {
		if _, err := ParseBingNewsQuery(raw); err == nil {
			t.Fatal("invalid query accepted", raw)
		}
	}
	q, err := ParseBingNewsQuery(" [zh-CN] 大麻 查获 ")
	if err != nil || q != (BingNewsQuery{"zh-HK", "大麻 查获"}) {
		t.Fatal(q, err)
	}
	q, err = ParseBingNewsQuery("cannabis raid")
	if err != nil || q.Market != "en-US" {
		t.Fatal(q, err)
	}
	g := BingNews{Queries: DefaultBingNewsQueries, QueriesPerScan: 4}
	if g.batch(0)[0] != g.Queries[0] || g.batch(1)[0] != g.Queries[4] || g.batch(2)[0] != g.Queries[8] || g.batch(len(g.Queries))[0] != g.Queries[0] {
		t.Fatal("query rotation failed")
	}
	for _, body := range []string{"<html>challenge</html>", "not XML", "<feed/>", "<rss><channel>" + strings.Repeat("<item/>", 251) + "</channel></rss>"} {
		if _, err := parseBingFeed([]byte(body)); err == nil {
			t.Fatal("invalid feed accepted")
		}
	}
}

func TestBingChineseSearchUsesRSSMarket(t *testing.T) {
	legacy, err := ParseBingNewsQuery("[zh-CN] 大麻 查获")
	if err != nil {
		t.Fatal(err)
	}
	queries := []BingNewsQuery{legacy}
	for _, q := range DefaultBingNewsQueries {
		if q.Query == "大麻 查获" {
			queries = append(queries, q)
		}
	}
	if len(queries) != 2 {
		t.Fatal("Simplified Chinese default query missing")
	}
	g := BingNews{client: &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("mkt") != "zh-HK" || r.URL.Query().Get("q") != "大麻 查获" {
			t.Error("Chinese query must use the RSS-capable market without changing keywords", r.URL)
			return newsResponse(r, "<html>Bing homepage</html>"), nil
		}
		return newsResponse(r, "<rss><channel>"+bingTestItem("https://publisher.example/chinese", time.Now())+"</channel></rss>"), nil
	})}}
	for _, q := range queries {
		items, err := g.feed(context.Background(), q)
		if err != nil || len(items) != 1 || items[0].Link != "https://publisher.example/chinese" {
			t.Fatal(items, err)
		}
	}
}

func TestBingNonRSSResponseReportsFinalURL(t *testing.T) {
	g := BingNews{client: &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		res := newsResponse(r, "<html>Bing homepage</html>")
		res.Request = r.Clone(r.Context())
		res.Request.URL, _ = url.Parse("https://www.bing.com/")
		return res, nil
	})}}
	items, err := g.feed(context.Background(), BingNewsQuery{"en-US", "cannabis"})
	if len(items) != 0 || err == nil || !strings.Contains(err.Error(), "RSS unavailable for en-US (response URL https://www.bing.com/)") {
		t.Fatal(items, err)
	}
}

func TestBingSchedulerRepeatsAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	g := BingNews{Queries: []BingNewsQuery{{"en-US", "cannabis"}}, Interval: time.Millisecond, client: &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return newsResponse(r, "<rss><channel/></rss>"), nil
	})}}
	done := make(chan struct{})
	go func() { g.Run(ctx); close(done) }()
	select {
	case <-done:
		if calls != 2 {
			t.Fatal(calls)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler did not stop")
	}
}

// Explicit opt-in: checks every default feed without queuing URLs or starting AI.
func TestBingNewsLive(t *testing.T) {
	if os.Getenv("BING_NEWS_LIVE") != "1" {
		t.Skip("set BING_NEWS_LIVE=1 to test public RSS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	g := BingNews{}
	paced := false
	for _, q := range DefaultBingNewsQueries {
		if err := pause(ctx, &paced); err != nil {
			t.Fatal(err)
		}
		t.Run(q.Market+"/"+q.Query, func(t *testing.T) {
			items, err := g.feed(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			if q.Query == "cannabis seized" && len(items) == 0 {
				t.Fatal("control feed returned no articles")
			}
			for _, item := range items {
				if _, err := bingPublisher(item.Link); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("market %s: %d articles", q.Market, len(items))
		})
	}
}
