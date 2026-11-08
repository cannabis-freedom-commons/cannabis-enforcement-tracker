package harvest

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type newsTransport func(*http.Request) (*http.Response, error)

func (f newsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func newsResponse(r *http.Request, b string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(b)), Request: r}
}
func redirectResponse(raw string) string {
	inner, _ := json.Marshal([]string{"garturlres", raw})
	outer, _ := json.Marshal([]any{[]any{"wrb.fr", "Fbv4je", string(inner)}})
	return ")]}'\n\n" + string(outer)
}
func TestGoogleRedirectParsing(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/private", "javascript:alert(1)", "https://news.google.com/articles/still-wrapped"} {
		if _, err := parseGoogleRedirect([]byte(redirectResponse(raw))); err == nil {
			t.Fatal("unsafe/unresolved URL accepted", raw)
		}
	}
	raw := "https://publisher.example/story?utm_source=google&edition=uk"
	got, err := parseGoogleRedirect([]byte(redirectResponse(raw)))
	if err != nil || got != "https://publisher.example/story?edition=uk" {
		t.Fatal(got, err)
	}
	if _, err = parseGoogleRedirect([]byte(")]}'\ninvalid")); err == nil {
		t.Fatal("invalid redirect accepted")
	}
}
func TestGoogleScanResolvesAndDurablyDeduplicates(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "news.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	var rpcCalls int
	client := &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == "/rss/search":
			if !strings.Contains(r.URL.Query().Get("q"), "cannabis") {
				t.Error("query missing")
			}
			return newsResponse(r, `<rss><channel><item><link>https://news.google.com/rss/articles/first</link></item><item><link>https://news.google.com/rss/articles/second</link></item></channel></rss>`), nil
		case strings.HasPrefix(r.URL.Path, "/rss/articles/"):
			id := strings.TrimPrefix(r.URL.Path, "/rss/articles/")
			return newsResponse(r, `<html><div data-n-a-sg="signature" data-n-a-ts="1793362567" data-n-a-id="`+id+`"></div></html>`), nil
		default:
			rpcCalls++
			if r.Method != "POST" {
				t.Error("redirect request must be POST")
			}
			r.ParseForm()
			if !strings.Contains(r.Form.Get("f.req"), "garturlreq") {
				t.Error("missing redirect payload")
			}
			return newsResponse(r, redirectResponse("https://publisher.example/cannabis-raid")), nil
		}
	})}
	g := GoogleNews{Store: s, Queries: []GoogleNewsQuery{{"US:en", "cannabis raid when:1d"}}, MaxArticles: 5, client: client}
	stats, err := g.Scan(context.Background())
	if err != nil || stats.Queued != 1 || stats.Known != 1 {
		t.Fatal(stats, err)
	}
	stats, err = g.Scan(context.Background())
	if err != nil || stats.Queued != 0 || stats.Known != 2 || rpcCalls != 2 {
		t.Fatal("repeated scan re-resolved or requeued candidates", stats, rpcCalls, err)
	}
	var jobs, posts int
	var raw, origin string
	s.DB.QueryRow("SELECT count(*) FROM jobs WHERE kind='research'").Scan(&jobs)
	s.DB.QueryRow("SELECT count(*) FROM outbox").Scan(&posts)
	s.DB.QueryRow("SELECT original_url,origin FROM submissions").Scan(&raw, &origin)
	if jobs != 1 || posts != 0 || origin != "rss" || raw != "https://publisher.example/cannabis-raid" {
		t.Fatal(jobs, posts, raw, origin)
	}
}
func TestGoogleScanDoesNotMarkUnresolvedCandidateSeen(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "news.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	calls := 0
	g := GoogleNews{Store: s, Queries: []GoogleNewsQuery{{"US:en", "cannabis"}}, client: &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/rss/search" {
			return newsResponse(r, `<rss><channel><item><link>https://news.google.com/rss/articles/broken</link></item></channel></rss>`), nil
		}
		calls++
		return newsResponse(r, "Google layout changed"), nil
	})}}
	for range 2 {
		stats, err := g.Scan(context.Background())
		if err != nil || stats.Unresolved != 1 || stats.Queued != 0 {
			t.Fatal(stats, err)
		}
	}
	if calls != 2 {
		t.Fatal("unresolved candidate must be retried", calls)
	}
}
func TestGoogleSchedulerRepeatsAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	g := GoogleNews{Queries: []GoogleNewsQuery{{"US:en", "cannabis"}}, Interval: time.Millisecond, client: &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return newsResponse(r, `<rss><channel/></rss>`), nil
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

func TestGoogleNewsQueryEditions(t *testing.T) {
	q, err := ParseGoogleNewsQuery(" [IN:hi] (गांजा OR चरस) when:1d ")
	if err != nil || q != (GoogleNewsQuery{"IN:hi", "(गांजा OR चरस) when:1d"}) {
		t.Fatal(q, err)
	}
	if p := q.params(); p.Get("hl") != "hi" || p.Get("gl") != "IN" || p.Get("ceid") != "IN:hi" {
		t.Fatal(p)
	}
	if p := (GoogleNewsQuery{"HK:zh-Hant", "大麻"}).params(); p.Get("hl") != "zh-HK" || p.Get("gl") != "HK" {
		t.Fatal(p)
	}
	for _, lang := range []string{"gu", "pa"} {
		if p := (GoogleNewsQuery{"IN:" + lang, "cannabis"}).params(); p.Get("hl") != lang+"-IN" || p.Get("ceid") != "IN:"+lang {
			t.Fatal(p)
		}
	}
	if p := (GoogleNewsQuery{"KE:en", "bhang"}).params(); p.Get("hl") != "en-KE" {
		t.Fatal(p)
	}
	if p := (GoogleNewsQuery{"PT:pt-150", "liamba"}).params(); p.Get("hl") != "pt-PT" || p.Get("ceid") != "PT:pt-150" {
		t.Fatal(p)
	}
	if q, err = ParseGoogleNewsQuery("cannabis when:1d"); err != nil || q.Edition != "US:en" {
		t.Fatal(q, err)
	}
	for _, bad := range []string{"[IN:hi", "[india] ganja", "[IN:hi]", "[IN:hi&x=1] ganja"} {
		if _, err = ParseGoogleNewsQuery(bad); err == nil {
			t.Fatal("invalid query accepted", bad)
		}
	}
	if len(DefaultGoogleNewsQueries) > MaxGoogleNewsQueries {
		t.Fatal("default set exceeds configured query limit")
	}
	editions := map[GoogleNewsQuery]bool{}
	for _, q := range DefaultGoogleNewsQueries {
		if !googleEdition.MatchString(q.Edition) || !(strings.HasSuffix(q.Query, "when:1d") || strings.HasSuffix(q.Query, "when:7d")) || len(q.Query) > 1000 || editions[q] {
			t.Fatal("invalid default query", q)
		}
		editions[q] = true
	}
}

func TestGoogleDefaultsReachSparseCountriesAndRegionalNames(t *testing.T) {
	for _, want := range []struct{ edition, place, term string }{
		{"ZA:en", "Malawi", "chamba"},
		{"KE:en", "Tanzania", "bangi"},
		{"FR:fr", "Madagascar", "rongony"},
		{"FR:fr", "La Réunion", "zamal"},
		{"PT:pt-150", "Angola", "liamba"},
		{"SA:ar", "البحرين", "الحشيش"},
		{"IN:en", "Maldives", "ganja"},
		{"SG:en", "Laos", "marijuana"},
		{"RU:ru", "Узбекистан", "анаша"},
		{"GB:en", "Albania", "cannabis"},
		{"MX:es-419", "Uruguay", "marihuana"},
		{"US:en", "Belize", "ganja"},
		{"AU:en", "Tuvalu", "marijuana"},
	} {
		t.Run(want.place, func(t *testing.T) {
			for _, q := range DefaultGoogleNewsQueries {
				if q.Edition == want.edition && strings.Contains(q.Query, want.place) && strings.Contains(q.Query, want.term) && strings.HasSuffix(q.Query, "when:7d") {
					return
				}
			}
			t.Fatal("missing country-targeted, seven-day query", want)
		})
	}
	for _, term := range []string{"weed", "hemp", "THC gummies", "hash oil", "CBD flower"} {
		found := false
		for _, q := range DefaultGoogleNewsQueries {
			found = found || strings.Contains(q.Query, term)
		}
		if !found {
			t.Fatal("missing cannabis vocabulary", term)
		}
	}
	// Geographic reach should start on the first scan, including after a restart.
	first := (GoogleNews{Queries: DefaultGoogleNewsQueries, QueriesPerScan: 12}).batch(0)
	for _, ed := range []string{"CA:en", "CA:fr", "AU:en", "NZ:en", "IE:en", "AR:es-419", "CO:es-419", "CL:es-419", "PE:es-419", "VE:es-419", "CU:es-419", "AT:de", "CH:de", "CH:fr", "BE:fr", "BE:nl", "EE:et", "MY:en", "AE:ar", "GH:en", "UG:en", "TZ:en", "ZW:en", "ET:en", "NA:en", "BW:en", "SN:fr"} {
		found := false
		for _, q := range DefaultGoogleNewsQueries {
			found = found || q.Edition == ed
		}
		if !found {
			t.Fatal("missing native edition", ed)
		}
	}
	editions := map[string]bool{}
	for _, q := range first {
		editions[q.Edition] = true
	}
	if len(editions) < 8 {
		t.Fatal("first scan is dominated by too few editions", editions)
	}
}
func TestGoogleScanSharesBudgetAcrossEditions(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "news.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	client := &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == "/rss/search":
			ed := strings.ReplaceAll(r.URL.Query().Get("ceid"), ":", "-")
			items := ""
			for i := range 3 {
				items += "<item><link>https://news.google.com/rss/articles/" + ed + "-" + string(rune('a'+i)) + "</link></item>"
			}
			return newsResponse(r, "<rss><channel>"+items+"</channel></rss>"), nil
		case strings.HasPrefix(r.URL.Path, "/rss/articles/"):
			id := strings.TrimPrefix(r.URL.Path, "/rss/articles/")
			return newsResponse(r, `<html><div data-n-a-sg="s" data-n-a-ts="1793362567" data-n-a-id="`+id+`"></div></html>`), nil
		default:
			r.ParseForm()
			var rows [][]any
			json.Unmarshal([]byte(r.Form.Get("f.req")), &rows)
			var inner []any
			json.Unmarshal([]byte(rows[0][0].([]any)[1].(string)), &inner)
			return newsResponse(r, redirectResponse("https://publisher.example/"+inner[2].(string))), nil
		}
	})}
	g := GoogleNews{Store: s, Queries: []GoogleNewsQuery{{"US:en", "cannabis"}, {"IN:hi", "गांजा"}}, MaxArticles: 2, client: client}
	stats, err := g.Scan(context.Background())
	if err != nil || stats.Found != 6 || stats.Queued != 2 {
		t.Fatal(stats, err)
	}
	var editions []string
	rows, _ := s.DB.Query("SELECT json_extract(payload,'$.edition') FROM jobs WHERE kind='research' ORDER BY json_extract(payload,'$.edition')")
	for rows.Next() {
		var e string
		rows.Scan(&e)
		editions = append(editions, e)
	}
	rows.Close()
	if strings.Join(editions, ",") != "IN:hi,US:en" {
		t.Fatal("budget not shared across editions", editions)
	}
}
func TestGoogleQueriesRotate(t *testing.T) {
	g := GoogleNews{Queries: []GoogleNewsQuery{{"US:en", "a"}, {"US:en", "b"}, {"US:en", "c"}}, QueriesPerScan: 2}
	if b := g.batch(0); b[0].Query != "a" || b[1].Query != "b" {
		t.Fatal(b)
	}
	if b := g.batch(1); b[0].Query != "c" || b[1].Query != "a" {
		t.Fatal(b)
	}
	g.QueriesPerScan = 0
	if len(g.batch(5)) != 3 {
		t.Fatal("all queries expected")
	}
}

// Validate new locales and query syntax against Google's public RSS without
// resolving articles, touching the operational queue or starting AI research.
func TestGoogleNewsLiveFeeds(t *testing.T) {
	if os.Getenv("GOOGLE_NEWS_LIVE") != "1" {
		t.Skip("set GOOGLE_NEWS_LIVE=1 to check public RSS editions")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	g := GoogleNews{}
	paced := false
	total, feeds := 0, 0
	for _, q := range DefaultGoogleNewsQueries {
		t.Run(q.Edition+"/"+q.Query, func(t *testing.T) {
			if err := pause(ctx, &paced); err != nil {
				t.Fatal(err)
			}
			b, final, err := g.request(ctx, "https://news.google.com/rss/search?"+q.params().Encode(), nil)
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(final)
			if err != nil {
				t.Fatal(err)
			}
			if got := u.Query().Get("ceid"); got != q.Edition {
				t.Fatalf("edition redirected from %s to %s", q.Edition, got)
			}
			var feed struct {
				XMLName xml.Name `xml:"rss"`
				Channel struct {
					Language string `xml:"language"`
				} `xml:"channel"`
			}
			if err := xml.Unmarshal(b, &feed); err != nil {
				t.Fatal(err)
			}
			links, err := ParseFeed(b)
			if err != nil {
				t.Fatal(err)
			}
			feeds++
			total += len(links)
			t.Logf("edition=%s language=%s results=%d", q.Edition, feed.Channel.Language, len(links))
		})
	}
	if total == 0 {
		t.Fatal("no results across the default query set")
	}
	t.Logf("%d query feeds returned %d candidates (not queued)", feeds, total)
}

func TestGoogleDefaultRotationCoversAllQueries(t *testing.T) {
	g := GoogleNews{Queries: DefaultGoogleNewsQueries, QueriesPerScan: 8}
	seen := map[GoogleNewsQuery]bool{}
	scans := (len(g.Queries) + g.QueriesPerScan - 1) / g.QueriesPerScan
	for n := range scans {
		batch := g.batch(n)
		if len(batch) != g.QueriesPerScan {
			t.Fatal("unexpected scan workload", len(batch))
		}
		for _, q := range batch {
			seen[q] = true
		}
	}
	if len(seen) != len(g.Queries) {
		t.Fatal("rotation omitted queries", len(seen), len(g.Queries))
	}
	// Worst case per scan: the 2m interval plus 20% jitter plus the 5m scan timeout.
	worst := time.Duration(scans) * (2*time.Minute*6/5 + 5*time.Minute)
	if worst >= 24*time.Hour {
		t.Fatal("default rotation exceeds query lookback", worst)
	}
	if len(g.Queries) > MaxGoogleNewsQueries {
		t.Fatal("defaults exceed the query bound", len(g.Queries))
	}
	t.Logf("%d queries rotate in %d scans (at most %s at defaults)", len(g.Queries), scans, worst)
}
