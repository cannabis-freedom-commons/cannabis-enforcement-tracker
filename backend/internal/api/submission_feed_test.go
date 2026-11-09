package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestSubmissionListAndValidation(t *testing.T) {
	a, h := server(t)
	id, err := a.Store.Accept(context.Background(), store.Intake{Origin: "gdelt", URL: "https://news.example/story", CanonicalURL: "https://news.example/story", Note: "Public"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/submissions", "/v1/submissions?origin=gdelt&status=queued", "/v1/submissions?ids=" + id} {
		w := request(h, "GET", path, "", "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), id) {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/v1/submissions?cursor=bad", "/v1/submissions?cursor=0", "/v1/submissions?status=unknown", "/v1/submissions?origin=unknown", "/v1/submissions?ids=", "/v1/submissions?sort=nope", "/v1/submissions?order=sideways"} {
		w := request(h, "GET", path, "", "")
		if w.Code != 400 {
			t.Fatal(path, w.Code)
		}
	}
	w := request(h, "GET", "/v1/submissions?origin=rss", "", "")
	var data store.SubmissionPage
	if err = json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data.Total != 0 || len(data.Submissions) != 0 {
		t.Fatal(data)
	}
}
func TestWebSocketAnnouncesIntakeTransitionsAndDeliveryChanges(t *testing.T) {
	a, h := server(t)
	httpServer := httptest.NewUnstartedServer(Instrument(h)) // as the backend serves it
	httpServer.Config.ReadTimeout = 500 * time.Millisecond
	httpServer.Config.WriteTimeout = 500 * time.Millisecond
	httpServer.Start()
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/v1/submissions/stream", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://tracker.example"}}})
	if err != nil {
		t.Fatal(err, res)
	}
	defer conn.CloseNow()
	var message struct {
		Type     string `json:"type"`
		Revision string `json:"revision"`
	}
	if err = wsjson.Read(ctx, conn, &message); err != nil {
		t.Fatal(err)
	}
	if message.Type != "submissions_changed" {
		t.Fatal(message)
	}
	previous := message.Revision
	id, err := a.Store.Accept(ctx, store.Intake{Origin: "rss", URL: "https://news.example/live", CanonicalURL: "https://news.example/live"})
	if err != nil {
		t.Fatal(err)
	}
	if err = wsjson.Read(ctx, conn, &message); err != nil {
		t.Fatal(err)
	}
	if message.Revision == previous {
		t.Fatal("intake notification missing")
	}
	previous = message.Revision
	v, _ := a.Store.Get(ctx, id)
	if err = a.Store.Transition(ctx, v.CaseID, "researching", "Live progress.", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = wsjson.Read(ctx, conn, &message); err != nil {
		t.Fatal(err)
	}
	if message.Revision == previous {
		t.Fatal("transition notification missing")
	}
	previous = message.Revision
	if _, err = a.Store.DB.ExecContext(ctx, "UPDATE submissions SET reply_event_id=? WHERE id=?", strings.Repeat("a", 64), id); err != nil {
		t.Fatal(err)
	}
	if err = wsjson.Read(ctx, conn, &message); err != nil {
		t.Fatal(err)
	}
	if message.Revision == previous {
		t.Fatal("reply notification missing")
	}
}
func TestWebSocketRejectsUnapprovedOrigins(t *testing.T) {
	_, h := server(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, origin := range []string{"https://evil.example", "http://tracker.example"} {
		conn, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/submissions/stream", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{origin}}})
		if conn != nil {
			conn.CloseNow()
		}
		if err == nil || res == nil || res.StatusCode != 403 {
			t.Fatal(origin, res, err)
		}
	}
}
