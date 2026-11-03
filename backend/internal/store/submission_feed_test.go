package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSubmissionFeedPagesEveryOriginWithoutPrivateData(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	origins := []string{"web", "nostr", "rss", "gdelt", "api"}
	ids := []string{}
	for i := 0; i < 57; i++ {
		id, err := s.Accept(ctx, Intake{Origin: origins[i%len(origins)], URL: fmt.Sprintf("https://news.example/%d", i), CanonicalURL: fmt.Sprintf("https://news.example/%d", i), Note: "A public note", Provenance: "PRIVATE: research provenance"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	page, err := s.ListSubmissions(ctx, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 57 || len(page.Submissions) != 50 || page.NextCursor == "" || page.Submissions[0].ID != ids[56] {
		t.Fatal("incorrect page", page)
	}
	if _, err = ParseSubmissionCursor(page.NextCursor); err != nil {
		t.Fatal(err)
	}
	// A new arrival cannot shift an older cursor page or hide same-second receipts.
	if _, err = s.Accept(ctx, Intake{Origin: "web", URL: "https://news.example/new", CanonicalURL: "https://news.example/new"}); err != nil {
		t.Fatal(err)
	}
	older, err := s.ListSubmissions(ctx, page.NextCursor, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Submissions) != 7 || older.NextCursor != "" || older.Total != 58 || older.Submissions[6].ID != ids[0] {
		t.Fatal("unstable pagination", older)
	}
	seen := map[string]bool{}
	for _, v := range append(page.Submissions, older.Submissions...) {
		if seen[v.ID] {
			t.Fatal("duplicate page receipt")
		}
		seen[v.ID] = true
	}
	if len(seen) != 57 {
		t.Fatal("missing receipts")
	}
	b, _ := json.Marshal(page)
	if strings.Contains(string(b), "PRIVATE") {
		t.Fatal("private provenance leaked")
	}
	rss, err := s.ListSubmissions(ctx, "", "queued", "rss", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if rss.Total != 11 {
		t.Fatal(rss.Total)
	}
	for _, v := range rss.Submissions {
		if v.Origin != "rss" || v.Status != "queued" {
			t.Fatal(v)
		}
	}
	if _, err = s.ListSubmissions(ctx, "", "bad", "", "", ""); err == nil {
		t.Fatal("invalid filter accepted")
	}
}
func TestFeedRevisionCoversSharedCasesAndNostrLinks(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	before, _ := s.FeedVersion(ctx)
	id := accept(t, s, false)
	other := legacyReceipt(t, s, id)
	after, _ := s.FeedVersion(ctx)
	if before == after {
		t.Fatal("intake did not invalidate register")
	}
	v, _ := s.Get(ctx, id)
	if err := s.Transition(ctx, v.CaseID, "researching", "Research in progress.", "", ""); err != nil {
		t.Fatal(err)
	}
	page, err := s.ListSubmissions(ctx, "", "researching", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Revision == after {
		t.Fatal("case change missing from feed")
	}
	if page.Submissions[0].ID != other {
		t.Fatal("status change reordered receipt")
	}
	previous := page.Revision
	if _, err = s.DB.Exec("UPDATE submissions SET gateway_event_id=? WHERE id=?", strings.Repeat("a", 64), id); err != nil {
		t.Fatal(err)
	}
	next, _ := s.FeedVersion(ctx)
	if next == previous {
		t.Fatal("Nostr update did not invalidate register")
	}
}

func TestSubmissionFeedIncludesCumulativeUsageAndBudgetResume(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	id := accept(t, s, false)
	other := legacyReceipt(t, s, id)
	v, _ := s.Get(ctx, id)
	for i := 0; i < 2; i++ {
		run, err := s.Reserve(ctx, v.CaseID, "private-workspace", 1, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec("UPDATE research_runs SET input_tokens=?,output_tokens=? WHERE id=?", 100+i, 20+i, run); err != nil {
			t.Fatal(err)
		}
	}
	job, err := s.Claim(ctx, "jobs", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(ctx, v.CaseID, "classifying", "The source is checked.", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, "jobs", job, ErrBudget); err != nil {
		t.Fatal(err)
	}
	page, err := s.ListSubmissions(ctx, "", "budget_wait", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Submissions) != 2 {
		t.Fatal(page)
	}
	for _, entry := range page.Submissions {
		if entry.InputTokens != 201 || entry.OutputTokens != 41 || entry.RunCount != 2 || entry.Status != "budget_wait" || entry.ResumeAt <= time.Now().Unix() || entry.LastStage != "classifying" {
			t.Fatal(entry)
		}
	}
	detail, err := s.Get(ctx, other)
	if err != nil || detail.Status != "budget_wait" || detail.ResumeAt != page.Submissions[0].ResumeAt || !strings.Contains(detail.Explanation, "No new AI run") {
		t.Fatal(detail, err)
	}
}

func TestSubmissionFeedOrdersByLatestUpdate(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	older, err := s.Accept(ctx, Intake{Origin: "web", URL: "https://news.example/older", CanonicalURL: "https://news.example/older"})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := s.Accept(ctx, Intake{Origin: "web", URL: "https://news.example/newer", CanonicalURL: "https://news.example/newer"})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Get(ctx, older)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE cases SET updated_at=? WHERE id=?", time.Now().Unix()+60, v.CaseID); err != nil {
		t.Fatal(err)
	}
	page, err := s.ListSubmissions(ctx, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Submissions) != 2 || page.Submissions[0].ID != older || page.Submissions[1].ID != newer {
		t.Fatal(page.Submissions)
	}
	if page.Submissions[0].UpdatedAt <= page.Submissions[1].UpdatedAt {
		t.Fatal(page.Submissions)
	}
	received, err := s.ListSubmissions(ctx, "", "", "", "received", "asc")
	if err != nil {
		t.Fatal(err)
	}
	if received.Submissions[0].ID != older || received.Submissions[1].ID != newer {
		t.Fatal(received.Submissions)
	}
	if _, err = s.ListSubmissions(ctx, FormatSubmissionCursor("received", "asc", "1", 1), "", "", "source", "asc"); err == nil {
		t.Fatal("cursor from another sort was accepted")
	}
	bySource, err := s.ListSubmissions(ctx, "", "", "", "source", "asc")
	if err != nil || bySource.Submissions[0].URL > bySource.Submissions[1].URL {
		t.Fatal(bySource.Submissions, err)
	}
}

func TestCommitRateUsesRecentPublicationsAcrossFiltersAndReceipts(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	now := time.Now().Unix()
	for i, age := range []int64{0, 30, 3590, 3601, -60} {
		url := fmt.Sprintf("https://news.example/rate-%d", i)
		id, err := s.Accept(ctx, Intake{Origin: "web", URL: url, CanonicalURL: url})
		if err != nil {
			t.Fatal(err)
		}
		v, err := s.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec("INSERT INTO local_publications VALUES(?,?,?,?)", v.CaseID, "sha", "reports/example.md", now-age); err != nil {
			t.Fatal(err)
		}
		legacyReceipt(t, s, id)
	}
	page, err := s.ListSubmissions(ctx, "", "failed", "rss", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || page.Summary.CommittedLastHour != 3 {
		t.Fatal(page.Summary, page.Total)
	}
	// An unrelated case update must not make an old publication recent again.
	if _, err := s.DB.Exec("UPDATE cases SET updated_at=?", now+1); err != nil {
		t.Fatal(err)
	}
	page, err = s.ListSubmissions(ctx, "", "", "", "", "")
	if err != nil || page.Summary.CommittedLastHour != 3 {
		t.Fatal(page.Summary, err)
	}
	if _, err := s.DB.Exec("DELETE FROM local_publications"); err != nil {
		t.Fatal(err)
	}
	page, err = s.ListSubmissions(ctx, "", "", "", "", "")
	if err != nil || page.Summary.CommittedLastHour != 0 {
		t.Fatal(page.Summary, err)
	}
}

func TestSubmissionCostSortIncludesConfiguredModelEstimates(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	// Deliberately insert out of API-cost order; variants and cached input
	// must be priced too, rather than sinking to the unknown-cost bucket.
	models := []string{"opencode-go/deepseek-v4.1-flash", "swancloud/muse-spark-1.3-contributor#high", "swancloud/deepseek-v4.1-flash", "opencode-go/muse-spark-1.3-contributor#xhigh"}
	ids := []string{}
	for i, model := range models {
		u := fmt.Sprintf("https://news.example/pricing-%d", i)
		id, err := s.Accept(ctx, Intake{Origin: "web", URL: u, CanonicalURL: u})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		v, _ := s.Get(ctx, id)
		run, err := s.Reserve(ctx, v.CaseID, "workspace", 1, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec("UPDATE research_runs SET model=?,input_tokens=500000,cache_read_tokens=500000,output_tokens=1000000,started_at=1793016000 WHERE id=?", model, run); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.ListSubmissions(ctx, "", "", "", "cost", "asc")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Submissions) != 4 || page.Submissions[0].ID != ids[2] || page.Submissions[1].ID != ids[3] || page.Submissions[2].ID != ids[1] || page.Submissions[3].ID != ids[0] {
		t.Fatal("incorrect estimated API-cost ordering", page.Submissions)
	}
	if page.Submissions[0].sortKey != "240000000" || page.Submissions[1].sortKey != "251000000" || page.Submissions[2].sortKey != "300000000" {
		t.Fatal("API value differs from displayed pricing", page.Submissions[0].sortKey, page.Submissions[1].sortKey, page.Submissions[2].sortKey)
	}
}

func TestSubmissionFeedPagesEverySortWithTies(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for i := 0; i < 57; i++ {
		u := fmt.Sprintf("https://news.example/%03d", i)
		if _, err := s.Accept(ctx, Intake{Origin: "rss", URL: u, CanonicalURL: u}); err != nil {
			t.Fatal(err)
		}
	}
	// Equal timestamps and empty usage force every sort to use the receipt tie-breaker.
	if _, err := s.DB.Exec("UPDATE cases SET created_at=1,updated_at=1; UPDATE submissions SET created_at=1"); err != nil {
		t.Fatal(err)
	}
	for sort := range submissionSorts {
		for _, order := range []string{"asc", "desc"} {
			t.Run(sort+"/"+order, func(t *testing.T) {
				seen := map[string]bool{}
				cursor := ""
				for pageNumber := 0; pageNumber < 2; pageNumber++ {
					page, err := s.ListSubmissions(ctx, cursor, "", "rss", sort, order)
					if err != nil {
						t.Fatal(err)
					}
					expected := 50
					if pageNumber == 1 {
						expected = 7
					}
					if len(page.Submissions) != expected || page.Total != 57 {
						t.Fatalf("incorrect bounded page: %d/%d", len(page.Submissions), page.Total)
					}
					for _, entry := range page.Submissions {
						if seen[entry.ID] {
							t.Fatalf("repeated receipt %s", entry.ID)
						}
						seen[entry.ID] = true
					}
					cursor = page.NextCursor
					if (pageNumber == 0) != (cursor != "") {
						t.Fatal("incorrect next cursor")
					}
				}
				if len(seen) != 57 {
					t.Fatal("missing receipts")
				}
			})
		}
	}
}
