package store

import (
	"context"
	"database/sql"
	"math"
	"time"
)

// SubmissionActivity is the submissions page's throughput chart: for each time slot of a window,
// the sources received, the cases committed (local_publications, as the commit rate), the AI
// tokens read and written (each run's counts spread evenly over its running time; a run still going,
// up to now), the cases closed without a report, and how many cases were open (waiting or running) at the slot's end.
// The open count is replayed from the status history, from each case's state when the window began.
type SubmissionActivity struct {
	Window string             `json:"window"`
	Step   int64              `json:"step"` // seconds per slot
	From   int64              `json:"from"`
	To     int64              `json:"to"`
	Slots  []SubmissionSlot   `json:"slots"`
	Totals SubmissionSlotSums `json:"totals"`
}

type SubmissionSlot struct {
	At        int64 `json:"at"` // the slot's start
	Received  int64 `json:"received"`
	Committed int64 `json:"committed"`
	Closed    int64 `json:"closed"`     // ended without a report: duplicate, out of scope, unavailable… (not failed)
	Failed    int64 `json:"failed"`     // ended because processing failed
	Open      int64 `json:"open"`       // cases waiting or running at the slot's end
	TokensIn  int64 `json:"tokens_in"`  // read by the AI runs, cached reads included
	TokensOut int64 `json:"tokens_out"` // written, reasoning included
}

type SubmissionSlotSums struct {
	Received  int64 `json:"received"`
	Committed int64 `json:"committed"`
	Closed    int64 `json:"closed"`
	Failed    int64 `json:"failed"`
	TokensIn  int64 `json:"tokens_in"`
	TokensOut int64 `json:"tokens_out"`
}

// ActivityWindows are the windows the chart offers: their length and slot size.
var ActivityWindows = map[string]struct{ Slots, Step int64 }{
	"1h":  {60, 60},
	"24h": {96, 15 * 60},
}

// closedWithoutReport: the terminal statuses that are not a commit
func closedWithoutReport(status string) bool {
	return Terminal(status) && status != "committed" && status != "published"
}

func (s *Store) SubmissionActivity(ctx context.Context, window string, now time.Time) (SubmissionActivity, error) {
	w, ok := ActivityWindows[window]
	if !ok {
		w, window = ActivityWindows["1h"], "1h"
	}
	to := (now.Unix()/w.Step + 1) * w.Step // the current slot, still filling, is the last
	from := to - w.Slots*w.Step
	out := SubmissionActivity{Window: window, Step: w.Step, From: from, To: to, Slots: make([]SubmissionSlot, w.Slots)}
	for i := range out.Slots {
		out.Slots[i].At = from + int64(i)*w.Step
	}
	slot := func(at int64) int {
		if at < from || at >= to {
			return -1
		}
		return int((at - from) / w.Step)
	}
	tx, err := s.reader().BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()

	count := func(query string, add func(*SubmissionSlot)) error {
		rows, err := tx.QueryContext(ctx, query, from, to)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var at int64
			if err := rows.Scan(&at); err != nil {
				return err
			}
			if i := slot(at); i >= 0 {
				add(&out.Slots[i])
			}
		}
		return rows.Err()
	}
	if err = count(`SELECT created_at FROM submissions WHERE created_at>=? AND created_at<?`, func(s *SubmissionSlot) { s.Received++ }); err != nil {
		return out, err
	}
	if err = count(`SELECT created_at FROM local_publications WHERE created_at>=? AND created_at<?`, func(s *SubmissionSlot) { s.Committed++ }); err != nil {
		return out, err
	}

	if err = s.activityTokens(ctx, tx, &out, now.Unix()); err != nil {
		return out, err
	}

	// each case's status when the window began: its latest history row before then
	state := map[string]string{}
	rows, err := tx.QueryContext(ctx, `SELECT s.case_id, h.status FROM submission_history h JOIN submissions s ON s.id=h.submission_id
    WHERE h.id IN (SELECT max(id) FROM submission_history WHERE created_at<? GROUP BY submission_id) ORDER BY h.id`, from)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var cid, status string
		if err := rows.Scan(&cid, &status); err != nil {
			rows.Close()
			return out, err
		}
		state[cid] = status // ordered by id: the case's latest row wins
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return out, err
	}
	open := int64(0)
	for _, status := range state {
		if !Terminal(status) {
			open++
		}
	}
	// then every change in the window, in order; a case's several receipts count once
	rows, err = tx.QueryContext(ctx, `SELECT s.case_id, h.status, h.created_at FROM submission_history h JOIN submissions s ON s.id=h.submission_id
    WHERE h.created_at>=? AND h.created_at<? ORDER BY h.id`, from, to)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	next := 0
	for rows.Next() {
		var cid, status string
		var at int64
		if err := rows.Scan(&cid, &status, &at); err != nil {
			return out, err
		}
		i := slot(at)
		if i < 0 {
			continue
		}
		for ; next < i; next++ {
			out.Slots[next].Open = open
		}
		old, seen := state[cid]
		if old == status {
			continue
		}
		wasOpen, isOpen := seen && !Terminal(old), !Terminal(status)
		if isOpen && !wasOpen {
			open++
		} else if wasOpen && !isOpen {
			open--
		}
		if status == "failed" {
			out.Slots[i].Failed++
		} else if closedWithoutReport(status) {
			out.Slots[i].Closed++
		}
		state[cid] = status
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	// the slots after the last change, up to now; the future part of the current slot is not shown
	for ; next < len(out.Slots); next++ {
		out.Slots[next].Open = open
	}
	for _, s := range out.Slots {
		out.Totals.Received += s.Received
		out.Totals.Committed += s.Committed
		out.Totals.Closed += s.Closed
		out.Totals.Failed += s.Failed
		out.Totals.TokensIn += s.TokensIn
		out.Totals.TokensOut += s.TokensOut
	}
	return out, nil
}

// activityTokens spreads each AI run's tokens over the slots it ran in, in proportion to the time.
func (s *Store) activityTokens(ctx context.Context, tx *sql.Tx, out *SubmissionActivity, now int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT started_at, COALESCE(NULLIF(finished_at,0), ?), input_tokens+cache_read_tokens, output_tokens+reasoning_tokens
    FROM research_runs WHERE started_at>0 AND started_at<? AND COALESCE(NULLIF(finished_at,0), ?)>=?`, now, out.To, now, out.From)
	if err != nil {
		return err
	}
	defer rows.Close()
	in := make([]float64, len(out.Slots))
	written := make([]float64, len(out.Slots))
	for rows.Next() {
		var start, end, read, wrote int64
		if err := rows.Scan(&start, &end, &read, &wrote); err != nil {
			return err
		}
		if end <= start { // finished within the second: all of it then
			end = start + 1
		}
		span := float64(end - start)
		for i := range out.Slots {
			a, b := max(start, out.Slots[i].At), min(end, out.Slots[i].At+out.Step)
			if b > a {
				share := float64(b-a) / span
				in[i] += float64(read) * share
				written[i] += float64(wrote) * share
			}
		}
	}
	for i := range out.Slots {
		out.Slots[i].TokensIn, out.Slots[i].TokensOut = int64(math.Round(in[i])), int64(math.Round(written[i]))
	}
	return rows.Err()
}
