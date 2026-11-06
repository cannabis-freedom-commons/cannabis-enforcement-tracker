package worker

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func words(s string) map[string]bool { return research.StoryTokens(s) }

func TestRelatedStoryMatchesOneStoryAcrossOutlets(t *testing.T) {
	same := storyHeadline("Married couple in their 70s who grew £46,000 of cannabis sentenced | Lynn News", "", "Lynn News")
	other := storyHeadline("Married couple in their 70s who grew £46,000 of cannabis sentenced - Thetford & Brandon Times", "", "")
	if !relatedStory(words(same), words(other)) {
		t.Fatal("one story in two papers")
	}
	// the site's name is not part of the story; different quantities are different events
	a := storyHeadline("Three Thai nationals arrested with over 5kg of Hashish at BIA | Sri Lanka News", "", "Sri Lanka News")
	b := storyHeadline("Canadian national arrested with over 20kg of hashish at BIA | Sri Lanka News", "", "Sri Lanka News")
	if relatedStory(words(a), words(b)) {
		t.Fatal("two seizures at one airport")
	}
	if strings.Contains(a, "Sri Lanka News") {
		t.Fatal("site name kept:", a)
	}
}

func TestHoldBackPutsTheJobBackWithoutAnAttempt(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().Unix()
	for _, cid := range []string{"held", "running"} {
		if _, err = s.DB.Exec(`INSERT INTO cases(id,canonical_url,status,created_at,updated_at) VALUES(?,?,'classifying',?,?)`, cid, "https://x.example/"+cid, now, now); err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`INSERT INTO submissions(id,origin,original_url,canonical_url,case_id,created_at) VALUES(?,?,?,?,?,?)`, cid+"0123456789", "rss", "https://x.example/"+cid, "https://x.example/"+cid, cid, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.DB.Exec(`INSERT INTO jobs(id,kind,ref,status,available_at,attempts,lease_until,lease_token) VALUES('j1','research','held','running',?,1,?, 'tok')`, now, now+600); err != nil {
		t.Fatal(err)
	}
	w := &Worker{Store: s}
	j := store.Job{ID: "j1", Kind: "research", Ref: "held", Token: "tok", Attempts: 1}
	held, err := w.holdBack(ctx, j, "running")
	if err != nil || !held {
		t.Fatal(held, err)
	}
	var status string
	var attempts, available int64
	if err = s.DB.QueryRow("SELECT status,attempts,available_at FROM jobs WHERE id='j1'").Scan(&status, &attempts, &available); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || attempts != 0 || available < now+60 {
		t.Fatalf("job: %s, %d attempts, available in %d s", status, attempts, available-now)
	}
	var explanation string
	s.DB.QueryRow("SELECT explanation FROM cases WHERE id='held'").Scan(&explanation)
	if !strings.HasPrefix(explanation, "Waiting for a related submission") || !strings.Contains(explanation, "running") {
		t.Fatal(explanation)
	}
	// after maxHolds it runs anyway
	for i := 1; i < maxHolds; i++ {
		s.DB.Exec("UPDATE jobs SET status='running',lease_token='tok' WHERE id='j1'")
		if held, err = w.holdBack(ctx, j, "running"); err != nil || !held {
			t.Fatal(i, held, err)
		}
	}
	s.DB.Exec("UPDATE jobs SET status='running',lease_token='tok' WHERE id='j1'")
	if held, err = w.holdBack(ctx, j, "running"); err != nil || held {
		t.Fatal("held past the limit", held, err)
	}
}
