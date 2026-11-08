package harvest

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// Bing's public RSS search is independent of the retired Bing Search API.
// It is undocumented: invalid feeds fail closed and are retried on the next scan.
type BingNewsQuery struct{ Market, Query string }

var bingMarket = regexp.MustCompile(`^[a-z]{2,3}-[A-Z]{2}$`)

func ParseBingNewsQuery(raw string) (BingNewsQuery, error) {
	q := BingNewsQuery{Market: "en-US", Query: strings.TrimSpace(raw)}
	if strings.HasPrefix(q.Query, "[") {
		end := strings.Index(q.Query, "]")
		if end < 0 {
			return q, errors.New("unclosed Bing News market")
		}
		q.Market, q.Query = q.Query[1:end], strings.TrimSpace(q.Query[end+1:])
	}
	if !bingMarket.MatchString(q.Market) {
		return q, fmt.Errorf("invalid Bing News market %q", q.Market)
	}
	if q.Query == "" {
		return q, errors.New("empty Bing News query")
	}
	// zh-CN redirects news searches to Bing's HTML homepage. The Hong Kong RSS
	// market accepts the same Simplified Chinese keywords without translating them.
	if q.Market == "zh-CN" {
		q.Market = "zh-HK"
	}
	return q, nil
}

// Keep searches short: Bing RSS returned empty feeds for compound Boolean queries
// in live testing, while simple cannabis/action searches returned publisher URLs.
// Every market below returned a feed with recent items in a November 2026 probe.
const MaxBingNewsQueries = 100

var DefaultBingNewsQueries = []BingNewsQuery{
	{"en-US", "cannabis seized"},
	{"en-US", "marijuana arrested"},
	{"en-US", "cannabis sentenced"},
	{"en-US", "illegal marijuana grow"},
	{"en-GB", "cannabis raid"},
	{"en-GB", "cannabis factory"},
	{"en-IE", "cannabis seized"},
	{"en-CA", "cannabis seized"},
	{"fr-CA", "cannabis saisie"},
	{"en-AU", "cannabis seized"},
	{"en-NZ", "cannabis police"},
	{"en-IN", "ganja seized"},
	{"en-PH", "marijuana arrested"},
	{"en-MY", "ganja seized"},
	{"en-ID", "marijuana arrested"},
	{"en-ZA", "dagga arrested"},
	{"en-NG", "NDLEA cannabis"},
	{"en-KE", "bhang police"},
	{"en-US", "China cannabis seized"},
	{"en-US", "Iran hashish"},
	{"en-US", `"North Korea" marijuana`},
	{"en-US", "Caribbean marijuana seized"},
	{"es-ES", "marihuana incautan"},
	{"es-ES", "hachís Guardia Civil"},
	{"es-MX", "marihuana aseguran"},
	{"es-AR", "marihuana secuestro"},
	{"es-CO", "marihuana incautan"},
	{"es-CL", "marihuana detenidos"},
	{"pt-BR", "maconha apreendida"},
	{"pt-PT", "haxixe apreendido"},
	{"fr-FR", "cannabis saisie"},
	{"fr-BE", "cannabis saisie"},
	{"nl-BE", "cannabis aangehouden"},
	{"nl-NL", "hennepkwekerij"},
	{"de-DE", "Cannabis sichergestellt"},
	{"de-AT", "Cannabis sichergestellt"},
	{"de-CH", "Cannabis Polizei"},
	{"it-IT", "marijuana sequestro"},
	{"pl-PL", "marihuana zatrzymali"},
	{"sv-SE", "cannabis gripen"},
	{"nb-NO", "hasj beslag"},
	{"da-DK", "hash politi"},
	{"fi-FI", "kannabis poliisi"},
	{"tr-TR", "esrar ele geçirildi"},
	{"ru-RU", "гашиш изъяли"},
	{"ru-RU", "марихуана задержали"},
	{"ar-SA", "حشيش ضبط"},
	{"ar-EG", "حشيش ضبط"},
	{"ja-JP", "大麻 逮捕"},
	{"ko-KR", "대마 검거"},
	{"zh-HK", "大麻 查获"},
	{"zh-HK", "大麻 檢獲"},
	{"zh-TW", "大麻 查獲"},
	{"id-ID", "ganja ditangkap"},
	{"th-TH", "กัญชา จับกุม"},
	{"vi-VN", "cần sa bắt giữ"},
	// Eradication and countries without a local Bing market (Sri Lanka, Nepal fall back
	// to other markets; es-PY, es-BO, es-PE and es-HN return local publishers).
	{"en-US", "marijuana eradication"},
	{"en-IN", "cannabis cultivation destroyed"},
	{"en-IN", "Sri Lanka cannabis"},
	{"en-IN", "Nepal ganja"},
	{"id-ID", "ladang ganja"},
	{"id-ID", "ganja dimusnahkan"},
	{"es-MX", "plantío marihuana"},
	{"pt-BR", "pés de maconha"},
	{"ru-RU", "дикорастущей конопли"},
	{"es-PY", "marihuana"},
	{"es-BO", "marihuana"},
	{"es-PE", "marihuana"},
	{"es-HN", "marihuana"},
	{"en-US", "Guyana cannabis"},
	{"en-US", "Jamaica ganja"},
}

type BingNews struct {
	Store                       *store.Store
	Queries                     []BingNewsQuery
	Interval                    time.Duration
	MaxArticles, QueriesPerScan int
	QueueTarget                 int // hold new articles back while this many research jobs wait (0 = off)
	client                      *http.Client
}
type BingNewsStats struct {
	Found, Queued, Known, Unresolved, Skipped int
	Deferred                                  bool // the research queue was full; the batch is retried next scan
}
type bingItem struct {
	Link      string `xml:"link"`
	Published string `xml:"pubDate"`
}

func parseBingFeed(b []byte) ([]bingItem, error) {
	var f struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Items []bingItem `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if len(f.Channel.Items) > 250 {
		return nil, errors.New("Bing News feed exceeds candidate bound")
	}
	return f.Channel.Items, nil
}

// Decode the publisher parameter without fetching Bing's tracking redirect. Only
// recognized wrappers are unwrapped; unsafe destinations and Bing pages are refused.
func bingPublisher(raw string) (string, error) {
	normalized, err := safeurl.Normalize(raw)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(normalized)
	if u.Hostname() == "bing.com" || strings.HasSuffix(u.Hostname(), ".bing.com") {
		if u.Path != "/news/apiclick.aspx" {
			return "", errors.New("unrecognized Bing News wrapper")
		}
		normalized, err = safeurl.Normalize(u.Query().Get("url"))
		if err != nil {
			return "", err
		}
		u, _ = url.Parse(normalized)
	}
	if u.Hostname() == "bing.com" || strings.HasSuffix(u.Hostname(), ".bing.com") || u.Hostname() == "news.google.com" {
		return "", errors.New("discovery URL did not reach a publisher")
	}
	return normalized, nil
}

func (g BingNews) feed(ctx context.Context, q BingNewsQuery) ([]bingItem, error) {
	params := url.Values{"q": {q.Query}, "format": {"rss"}, "mkt": {q.Market}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.bing.com/news/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CannabisEnforcementTracker discovery)")
	client := g.client
	if client == nil {
		client = safeurl.Client()
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Bing News HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 2*1024*1024 {
		return nil, errors.New("Bing News response exceeds limit")
	}
	items, err := parseBingFeed(b)
	if err != nil {
		responseURL := req.URL.String()
		if res.Request != nil && res.Request.URL != nil {
			responseURL = res.Request.URL.Redacted()
		}
		return nil, fmt.Errorf("Bing News RSS unavailable for %s (response URL %s): %w", q.Market, responseURL, err)
	}
	return items, nil
}

func (g BingNews) Scan(ctx context.Context) (BingNewsStats, error) { return g.scan(ctx, g.Queries) }
func (g BingNews) scan(ctx context.Context, queries []BingNewsQuery) (stats BingNewsStats, err error) {
	max := g.MaxArticles
	if max <= 0 {
		max = 10
	}
	room, err := queueRoom(ctx, g.Store, g.QueueTarget)
	if err != nil {
		return stats, err
	}
	if room == 0 {
		stats.Deferred = true
		return stats, nil
	}
	max = min(max, room)
	results := make([][]bingItem, len(queries))
	paced := false
	for i, q := range queries {
		if err = pause(ctx, &paced); err != nil {
			return stats, err
		}
		slog.Info("Bing News search started", "market", q.Market, "query", q.Query)
		items, fetchErr := g.feed(ctx, q)
		if fetchErr != nil {
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			slog.Warn("Bing News search failed", "market", q.Market, "error", fetchErr)
			continue
		}
		results[i] = items
		stats.Found += len(items)
		slog.Info("Bing News search complete", "market", q.Market, "results", len(items))
	}
	// Round-robin admission keeps a busy market from consuming the whole budget.
	seen := map[string]bool{}
	now := time.Now()
	for rank, more := 0, true; more && stats.Queued < max; rank++ {
		more = false
		for i, q := range queries {
			if rank >= len(results[i]) {
				continue
			}
			more = true
			if stats.Queued >= max {
				break
			}
			item := results[i][rank]
			published, dateErr := http.ParseTime(item.Published)
			// RSS can return years-old hits. Undated, old and implausibly future hits
			// never enter the research queue. Publication dates are only discovery hints.
			if dateErr != nil || published.Before(now.Add(-7*24*time.Hour)) || published.After(now.Add(24*time.Hour)) {
				stats.Skipped++
				continue
			}
			publisher, resolveErr := bingPublisher(item.Link)
			if resolveErr != nil {
				stats.Unresolved++
				continue
			}
			if seen[publisher] {
				stats.Known++
				continue
			}
			seen[publisher] = true
			var exists bool
			if err = g.Store.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM cases WHERE canonical_url=? UNION ALL SELECT 1 FROM source_urls WHERE url=? OR final_url=?)", publisher, publisher, publisher).Scan(&exists); err != nil {
				return stats, err
			}
			if exists {
				stats.Known++
				continue
			}
			id, acceptErr := g.Store.Accept(ctx, store.Intake{Origin: "rss", URL: publisher, CanonicalURL: publisher, IdempotencyKey: "bing-news:" + hash(publisher), Provenance: store.JSON(map[string]any{"provider": "bing-news", "market": q.Market, "query": q.Query, "discovery_url": item.Link, "discovered_at": now.Unix()})})
			if acceptErr != nil {
				return stats, acceptErr
			}
			stats.Queued++
			slog.Info("Bing News article queued", "url", publisher, "market", q.Market, "submission_id", id)
		}
	}
	return stats, nil
}
func (g BingNews) batch(n int) []BingNewsQuery {
	size := g.QueriesPerScan
	if size <= 0 || size >= len(g.Queries) {
		return g.Queries
	}
	out := make([]BingNewsQuery, 0, size)
	for i := range size {
		out = append(out, g.Queries[(n*size+i)%len(g.Queries)])
	}
	return out
}
func (g BingNews) Run(ctx context.Context) {
	interval := g.Interval
	if interval <= 0 {
		interval = 3 * time.Minute
	}
	slog.Info("Bing News discovery enabled", "interval", interval.String(), "queries", len(g.Queries), "queries_per_scan", len(g.batch(0)), "max_new_articles", g.MaxArticles, "queue_target", g.QueueTarget)
	// Start offset from Google News so the two providers do not admit in the same minute.
	timer := time.NewTimer(min(30*time.Second, interval/4))
	defer timer.Stop()
	for n := 0; ; {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			started := time.Now()
			run, cancel := context.WithTimeout(ctx, 5*time.Minute)
			stats, err := g.scan(run, g.batch(n))
			cancel()
			if err != nil && ctx.Err() == nil {
				slog.Warn("Bing News scan interrupted", "error", err)
			}
			next := jitter(interval)
			if stats.Deferred {
				slog.Info("Bing News scan deferred; research queue full", "queue_target", g.QueueTarget, "next_scan_in", next.String())
			} else {
				n++
				slog.Info("Bing News scan complete", "found", stats.Found, "queued", stats.Queued, "known", stats.Known, "unresolved", stats.Unresolved, "skipped", stats.Skipped, "duration", time.Since(started).String(), "next_scan_in", next.String())
			}
			timer.Reset(next)
		}
	}
}
