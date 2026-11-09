package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// News discovery brings a story in batches: several outlets of one region report the same raid
// within minutes. Researched in parallel, the first to be committed makes the others' drafts
// stale and they are sent back for a review. Instead, a case whose page is about the same story as
// one already being researched waits for that one: when it runs, the report exists and the
// research finds it straight away.

var errHeldBack = errors.New("research held back for a related submission")

const (
	holdDelay = 3 * time.Minute
	maxHolds  = 8 // after that it runs anyway: a long chain of related stories must not starve it
)

// storyWords are the distinctive words of a page's headline and description: words found in many
// recent submissions' titles (in any language: "police", "marihuana", "detenido") say nothing
// about which story it is.
func (w *Worker) storyWords(ctx context.Context, text string) map[string]bool {
	common := w.commonTitleWords(ctx)
	out := map[string]bool{}
	for word := range research.StoryTokens(text) {
		if !common[word] {
			out[word] = true
		}
	}
	return out
}

var titleWords struct {
	sync.Mutex
	at     time.Time
	common map[string]bool
}

// commonTitleWords: the words in more than 1 in 40 (and at least 6) of the last three days'
// source titles, worked out at most every ten minutes.
func (w *Worker) commonTitleWords(ctx context.Context) map[string]bool {
	titleWords.Lock()
	defer titleWords.Unlock()
	if titleWords.common != nil && time.Since(titleWords.at) < 10*time.Minute {
		return titleWords.common
	}
	checks, err := w.backend().RecentSourceChecks(ctx)
	if err != nil {
		return titleWords.common
	}
	frequency, n := map[string]int{}, 0
	for _, raw := range checks {
		n++
		for word := range research.StoryTokens(sourceHeadline(raw)) {
			frequency[word]++
		}
	}
	limit := max(6, n/40)
	common := map[string]bool{}
	for word, f := range frequency {
		if f > limit {
			common[word] = true
		}
	}
	titleWords.at, titleWords.common = time.Now(), common
	return common
}

// sourceHeadline is a source check's title and description (cases.source_check)
func sourceHeadline(raw string) string {
	var check struct {
		Title    string `json:"title"`
		Metadata *struct {
			Description string `json:"description"`
			SiteName    string `json:"site_name"`
		} `json:"metadata"`
	}
	if json.Unmarshal([]byte(raw), &check) != nil {
		return ""
	}
	description, site := "", ""
	if check.Metadata != nil {
		description, site = check.Metadata.Description, check.Metadata.SiteName
	}
	return storyHeadline(check.Title, description, site)
}

// storyHeadline is a page's title and description without the site's name, which every story of
// that site shares: a trailing " | Site" or " - Site" part of the title, and the site name itself.
func storyHeadline(title, description, site string) string {
	for _, sep := range []string{" | ", " - ", " – ", " — "} {
		if i := strings.LastIndex(title, sep); i > 0 && len([]rune(title[i+len(sep):])) <= 40 {
			title = title[:i]
		}
	}
	text := title + " " + description
	if site = strings.TrimSpace(site); len(site) >= 3 {
		text = strings.ReplaceAll(text, site, " ")
	}
	return text
}

// relatedStory says whether two pages tell the same story: at least three distinctive words in
// common, making up at least half of the shorter one's, and no disagreement on the numbers (5 kg
// and 20 kg seized are two events).
func relatedStory(a, b map[string]bool) bool {
	shared, numbersA, numbersB, sharedNumbers := 0, 0, 0, 0
	for word := range a {
		number := strings.ContainsAny(word, "0123456789")
		if number {
			numbersA++
		}
		if b[word] {
			shared++
			if number {
				sharedNumbers++
			}
		}
	}
	for word := range b {
		if strings.ContainsAny(word, "0123456789") {
			numbersB++
		}
	}
	if numbersA > 0 && numbersB > 0 && sharedNumbers == 0 {
		return false
	}
	return shared >= 3 && float64(shared) >= 0.5*float64(min(len(a), len(b)))
}

// relatedRunning finds a case being researched (its AI run started, or its draft waiting to be
// committed) whose page tells the same story as this one's; "" when there is none.
func (w *Worker) relatedRunning(ctx context.Context, cid, headline string) (string, error) {
	mine := w.storyWords(ctx, headline)
	if len(mine) < 3 {
		return "", nil
	}
	checks, err := w.backend().ResearchingSourceChecks(ctx, cid)
	if err != nil {
		return "", err
	}
	var related []string
	for other, raw := range checks {
		if relatedStory(mine, w.storyWords(ctx, sourceHeadline(raw))) {
			related = append(related, other)
		}
	}
	sort.Strings(related)
	if len(related) == 0 {
		return "", nil
	}
	return related[0], nil
}

// holdBack puts the job back for a few minutes without counting it as an attempt, and says why on
// the request; it stops holding after maxHolds times.
func (w *Worker) holdBack(ctx context.Context, j store.Job, other string) (bool, error) {
	return w.backend().HoldBack(ctx, j, other)
}

func (b Local) HoldBack(ctx context.Context, j store.Job, other string) (bool, error) {
	var holds int
	if err := b.DB.QueryRowContext(ctx, `SELECT count(*) FROM submission_history h JOIN submissions s ON s.id=h.submission_id
    WHERE s.case_id=? AND h.explanation LIKE 'Waiting for a related submission%' AND s.rowid=(SELECT min(rowid) FROM submissions WHERE case_id=?)`, j.Ref, j.Ref).Scan(&holds); err != nil {
		return false, err
	}
	if holds >= maxHolds {
		return false, nil
	}
	var receipt string
	if err := b.DB.QueryRowContext(ctx, "SELECT id FROM submissions WHERE case_id=? ORDER BY created_at, rowid LIMIT 1", other).Scan(&receipt); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	tx, err := b.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now()
	res, err := tx.ExecContext(ctx, "UPDATE jobs SET status='queued',available_at=?,lease_until=0,lease_token='',attempts=max(0,attempts-1) WHERE id=? AND lease_token=? AND status='running'", now.Add(holdDelay).Unix(), j.ID, j.Token)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return false, errors.New("research lease lost before holding back")
	}
	why := "Waiting for a related submission to finish: another source about what looks like the same story is being researched. This one continues when it is done, so that it can be added to that report instead of being researched twice."
	if len(receipt) >= 7 {
		why = strings.Replace(why, "another source", "another source ("+receipt[:7]+")", 1)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE cases SET status='queued',explanation=?,updated_at=? WHERE id=?", why, now.Unix(), j.Ref); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO submission_history(submission_id,status,explanation,actor,created_at) SELECT id,'queued',?,'worker',? FROM submissions WHERE case_id=?", why, now.Unix(), j.Ref); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
