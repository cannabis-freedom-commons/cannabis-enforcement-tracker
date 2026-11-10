package api

import (
	"net/http"
	"time"
)

func (s *Server) aiStats(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	var since int64
	period := r.URL.Query().Get("period")
	switch period {
	case "", "all":
		period = "all"
	case "30d":
		since = now.Add(-30 * 24 * time.Hour).Unix()
	case "today":
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
	default:
		problem(w, 400, "invalid statistics period")
		return
	}
	buckets, err := s.Store.AIUsage(r.Context(), since)
	if err != nil {
		problem(w, 503, "AI usage is unavailable")
		return
	}
	durations, err := s.Store.AIDurations(r.Context(), since)
	if err != nil {
		problem(w, 503, "AI usage is unavailable")
		return
	}
	p := s.processingInfo(r)
	write(w, 200, map[string]any{"period": period, "since": since, "updated_at": now.Unix(), "usage": buckets, "durations": durations, "plan": p.Plan})
}
