package harvest

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func TestJitterStaysWithinTwentyPercent(t *testing.T) {
	for range 1000 {
		if d := jitter(time.Minute); d < 48*time.Second || d > 72*time.Second {
			t.Fatal("jitter out of range", d)
		}
	}
}

func queueJobs(t *testing.T, s *store.Store, n int) {
	t.Helper()
	for i := range n {
		u := fmt.Sprintf("https://waiting.example/%d", i)
		if _, err := s.Accept(context.Background(), store.Intake{Origin: "rss", URL: u, CanonicalURL: u, IdempotencyKey: "waiting:" + u}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscoveryHoldsBackWhileQueueIsFull(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "pacing.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	queueJobs(t, s, 3)
	searches := 0
	client := &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		searches++
		return newsResponse(r, `<rss><channel/></rss>`), nil
	})}
	google := GoogleNews{Store: s, Queries: []GoogleNewsQuery{{"US:en", "cannabis"}}, QueueTarget: 3, client: client}
	if stats, err := google.Scan(context.Background()); err != nil || !stats.Deferred {
		t.Fatal("Google scan should defer", stats, err)
	}
	bing := BingNews{Store: s, Queries: []BingNewsQuery{{"en-US", "cannabis"}}, QueueTarget: 3, client: client}
	if stats, err := bing.Scan(context.Background()); err != nil || !stats.Deferred {
		t.Fatal("Bing scan should defer", stats, err)
	}
	if searches != 0 {
		t.Fatal("deferred scans must not search", searches)
	}
}

func TestDiscoveryAdmitsOnlyQueueRoom(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "pacing.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	queueJobs(t, s, 2)
	now := time.Now()
	wrap := func(raw string) string { return "https://www.bing.com/news/apiclick.aspx?url=" + raw }
	items := ""
	for i := range 6 {
		items += bingTestItem(wrap(fmt.Sprintf("https://publisher.example/%d", i)), now)
	}
	client := &http.Client{Transport: newsTransport(func(r *http.Request) (*http.Response, error) {
		return newsResponse(r, "<rss><channel>"+items+"</channel></rss>"), nil
	})}
	g := BingNews{Store: s, Queries: []BingNewsQuery{{"en-US", "cannabis"}}, MaxArticles: 10, QueueTarget: 5, client: client}
	stats, err := g.Scan(context.Background())
	if err != nil || stats.Deferred || stats.Queued != 3 {
		t.Fatal("expected three admissions to fill the queue to its target", stats, err)
	}
	// The held-back candidates are offered again once the queue drains.
	if _, err := s.DB.Exec("UPDATE jobs SET status='done'"); err != nil {
		t.Fatal(err)
	}
	stats, err = g.Scan(context.Background())
	if err != nil || stats.Queued != 3 {
		t.Fatal("held-back candidates were not offered again", stats, err)
	}
}
