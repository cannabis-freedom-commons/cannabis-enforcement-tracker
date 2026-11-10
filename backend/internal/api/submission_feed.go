package api

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func (s *Server) submissions(w http.ResponseWriter, r *http.Request) {
	// Preserve the existing browser receipt batch lookup.
	if r.URL.Query().Has("ids") {
		s.batch(w, r)
		return
	}
	cursor := r.URL.Query().Get("cursor")
	sortBy, order := r.URL.Query().Get("sort"), r.URL.Query().Get("order")
	if !store.ValidSubmissionSort(sortBy, order) {
		problem(w, 400, "Invalid submission sort.")
		return
	}
	if _, err := store.ParseSubmissionCursor(cursor); err != nil {
		problem(w, 400, "Invalid submission cursor.")
		return
	}
	status, origin := r.URL.Query().Get("status"), r.URL.Query().Get("origin")
	if !store.ValidSubmissionFilter(status, origin) {
		problem(w, 400, "Invalid submission filter.")
		return
	}
	page, err := s.Store.ListSubmissions(r.Context(), cursor, status, origin, sortBy, order)
	if err != nil {
		problem(w, 503, "Submissions could not be loaded.")
		return
	}
	write(w, 200, struct {
		store.SubmissionPage
		Processing ProcessingInfo `json:"processing"`
	}{page, s.processingInfo(r)})
}

// Every connection reads only one indexed version row each second. Messages invalidate a
// bounded page, never broadcast the corpus, private job data or article/model evidence.
func (s *Server) submissionStream(w http.ResponseWriter, r *http.Request) {
	s.streamMu.Lock()
	if s.streamCount >= 128 {
		s.streamMu.Unlock()
		problem(w, 503, "Live updates are busy. Please retry shortly.")
		return
	}
	s.streamCount++
	s.streamMu.Unlock()
	defer func() { s.streamMu.Lock(); s.streamCount--; s.streamMu.Unlock() }()
	origins := []string{}
	for _, raw := range s.Config.Origins {
		if u, err := url.Parse(raw); err == nil {
			origins = append(origins, u.Host)
		}
	}
	// HTTP request deadlines must not carry over to a long-lived upgraded connection.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Time{})
	_ = controller.SetWriteDeadline(time.Time{})
	// Handler middleware already checks the complete Origin, including scheme and port.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: origins})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	ctx = conn.CloseRead(ctx)
	changes := time.NewTicker(time.Second)
	defer changes.Stop()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	previous := ""
	for {
		version, err := s.Store.FeedVersion(ctx)
		if err != nil {
			return
		}
		if version != previous {
			send, stop := context.WithTimeout(ctx, 5*time.Second)
			err = wsjson.Write(send, conn, map[string]string{"type": "submissions_changed", "revision": version})
			stop()
			if err != nil {
				return
			}
			previous = version
		}
		select {
		case <-ctx.Done():
			return
		case <-changes.C:
		case <-heartbeat.C:
			ping, stop := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Ping(ping)
			stop()
			if err != nil {
				return
			}
		}
	}
}
