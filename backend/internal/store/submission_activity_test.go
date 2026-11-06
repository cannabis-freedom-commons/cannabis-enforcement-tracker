package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSubmissionActivityReplaysTheQueue(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	T := time.Unix(1793990400, 0) // on a minute
	at := func(ago int64) int64 { return T.Unix() - ago }
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	receipt := func(cid, sid string, created int64) {
		exec(`INSERT OR IGNORE INTO cases(id,canonical_url,created_at,updated_at) VALUES(?,?,?,?)`, cid, "https://x.example/"+cid, created, created)
		exec(`INSERT INTO submissions(id,origin,original_url,canonical_url,case_id,created_at) VALUES(?,?,?,?,?,?)`, sid, "web", "https://x.example/"+cid, "https://x.example/"+cid, cid, created)
	}
	history := func(cid, status string, when int64) {
		exec(`INSERT INTO submission_history(submission_id,status,explanation,actor,created_at) SELECT id,?,'','worker',? FROM submissions WHERE case_id=?`, status, when, cid)
	}
	// a: open when the window begins, committed in it
	receipt("a", "a1", at(7200))
	history("a", "queued", at(7200))
	history("a", "researching", at(7100))
	history("a", "committed", at(1800))
	exec(`INSERT INTO local_publications(case_id,commit_sha,report_path,created_at) VALUES('a','abc','reports/a.md',?)`, at(1800))
	// d: finished long before
	receipt("d", "d1", at(9000))
	history("d", "queued", at(9000))
	history("d", "committed", at(8900))
	// c: one case, two receipts: received twice, open once
	receipt("c", "c1", at(1200))
	receipt("c", "c2", at(1200))
	history("c", "queued", at(1200))
	// b: received and turned away in the window
	receipt("b", "b1", at(600))
	history("b", "queued", at(600))
	history("b", "not_relevant", at(300))

	// a run of a: 120 s, 1,200 tokens read (cache included) and 240 written;
	// a run still going since 30 s ago, 300 read so far
	exec(`INSERT INTO research_runs(id,case_id,workspace,reserved_cents,created_at,status,started_at,finished_at,input_tokens,cache_read_tokens,output_tokens,reasoning_tokens) VALUES('r1','a','',0,?,'succeeded',?,?,1000,200,200,40)`, at(1900), at(1900), at(1780))
	exec(`INSERT INTO research_runs(id,case_id,workspace,reserved_cents,created_at,status,started_at,finished_at,input_tokens) VALUES('r2','c','',0,?,'running',?,0,300)`, at(30), at(30))
	a, err := s.SubmissionActivity(context.Background(), "1h", T)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Slots) != 60 || a.Step != 60 || a.To != T.Unix()+60 {
		t.Fatalf("window: %d slots, step %d, to %d", len(a.Slots), a.Step, a.To)
	}
	if a.Totals != (SubmissionSlotSums{Received: 3, Committed: 1, Closed: 1, TokensIn: 1500, TokensOut: 240}) {
		t.Fatalf("totals: %+v", a.Totals)
	}
	idx := func(ago int64) int { return int((at(ago) - a.From) / a.Step) }
	for ago, want := range map[int64]int64{3500: 1, 1860: 1, 1800: 0, 1200: 1, 600: 2, 300: 1, 0: 1} {
		if got := a.Slots[idx(ago)].Open; got != want {
			t.Errorf("open %d s ago: %d, want %d", ago, got, want)
		}
	}
	// 1900 s to 1780 s ago: slots of 1920–1860 (40 s of it), 1860–1800 (60 s), 1800–1740 (20 s)
	if in := [3]int64{a.Slots[idx(1900)].TokensIn, a.Slots[idx(1860)].TokensIn, a.Slots[idx(1800)].TokensIn}; in != [3]int64{400, 600, 200} {
		t.Errorf("tokens spread: %v", in)
	}
	if a.Slots[idx(30)].TokensIn != 300 {
		t.Errorf("running run: %d", a.Slots[idx(30)].TokensIn)
	}
	if a.Slots[idx(300)].Closed != 1 || a.Slots[idx(1800)].Committed != 1 || a.Slots[idx(1200)].Received != 2 {
		t.Errorf("slot counts: %+v %+v %+v", a.Slots[idx(300)], a.Slots[idx(1800)], a.Slots[idx(1200)])
	}
}
