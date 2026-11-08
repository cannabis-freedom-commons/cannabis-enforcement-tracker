package harvest

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

// Discovery feeds the research queue as a steady trickle instead of bursts: each scan
// admits at most its own budget and only as many articles as the waiting queue has
// room for. Candidates held back stay unseen and are offered again by a later scan
// while the search's lookback still returns them.

// queueRoom is how many more research jobs may wait before the queue reaches target.
// A target of 0 or less disables backpressure.
func queueRoom(ctx context.Context, s *store.Store, target int) (int, error) {
	if target <= 0 {
		return int(^uint(0) >> 1), nil
	}
	var waiting int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE kind='research' AND status='queued'").Scan(&waiting); err != nil {
		return 0, err
	}
	return max(0, target-waiting), nil
}

// jitter spreads scans by ±20% so providers do not fire in step with each other.
func jitter(d time.Duration) time.Duration {
	spread := int64(d) / 5
	if spread <= 0 {
		return d
	}
	return d - time.Duration(spread) + time.Duration(rand.Int64N(2*spread+1))
}
