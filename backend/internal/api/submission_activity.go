package api

import (
	"net/http"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

type cachedActivity struct {
	at   time.Time
	data store.SubmissionActivity
}

// the activity chart replays the status history: worked out at most twice a minute per window
const activityTTL = 30 * time.Second

func (s *Server) submissionActivity(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "1h"
	}
	if _, ok := store.ActivityWindows[window]; !ok {
		problem(w, 400, "Invalid activity window.")
		return
	}
	now := time.Now()
	s.activityMu.Lock()
	defer s.activityMu.Unlock() // one replay at a time; the others wait for its result
	if c, ok := s.activityCache[window]; ok && now.Sub(c.at) < activityTTL {
		write(w, 200, c.data)
		return
	}
	data, err := s.Store.SubmissionActivity(r.Context(), window, now)
	if err != nil {
		problem(w, 503, "Submission activity could not be loaded.")
		return
	}
	if s.activityCache == nil {
		s.activityCache = map[string]cachedActivity{}
	}
	s.activityCache[window] = cachedActivity{now, data}
	write(w, 200, data)
}
