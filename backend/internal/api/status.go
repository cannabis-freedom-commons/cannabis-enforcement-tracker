package api

import "net/http"

// status is the header's indicator: the service answers, whether AI processing is on, and how many
// requests are being worked on or waiting. One small query, cheap enough to poll from every page.
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	var running, queued int64
	err := s.Store.Reader().QueryRowContext(r.Context(), `SELECT
 coalesce(sum(status IN ('fetching','classifying','researching','verifying','drafted','awaiting_review','publishing','merged','deploying')),0),
 coalesce(sum(status IN ('queued','budget_wait')),0) FROM cases`).Scan(&running, &queued)
	if err != nil {
		problem(w, 503, "Status unavailable.")
		return
	}
	p := s.processingInfo(r.Context())
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, struct {
		Online     bool   `json:"online"`
		Processing bool   `json:"processing"`
		Reason     string `json:"reason,omitempty"`
		Running    int64  `json:"running"`
		Queued     int64  `json:"queued"`
	}{true, p.Enabled, p.Reason, running, queued})
}
