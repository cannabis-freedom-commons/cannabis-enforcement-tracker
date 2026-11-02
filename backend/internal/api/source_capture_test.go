package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func TestSourceScreenshotIsLocalAndBoundToReceipt(t *testing.T) {
	s, _ := server(t)
	s.Config.Addr = "127.0.0.1:8080"
	s.Config.Workspaces = t.TempDir()
	id, err := s.Store.Accept(context.Background(), store.Intake{Origin: "web", URL: "https://news.example/screenshot", CanonicalURL: "https://news.example/screenshot"})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := s.Store.Get(context.Background(), id)
	dir := filepath.Join(s.Config.Workspaces, "sources", v.CaseID, store.ID())
	os.MkdirAll(dir, 0700)
	png := []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0}
	if err = os.WriteFile(filepath.Join(dir, "page.png"), png, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec("INSERT INTO source_captures(id,case_id,url,artifact_dir,created_at) VALUES(?,?,?,?,?)", store.ID(), v.CaseID, v.URL, dir, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	send := func(remote string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/v1/submissions/"+id+"/source/screenshot", nil)
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if r := send("127.0.0.1:1234"); r.Code != 403 {
		t.Fatal("screenshots enabled outside development")
	}
	s.Config.DevControls = true
	if r := send("198.51.100.1:1234"); r.Code != 403 {
		t.Fatal("remote screenshot access allowed")
	}
	if r := send("127.0.0.1:1234"); r.Code != 200 || r.Header().Get("Content-Type") != "image/png" || len(r.Body.Bytes()) != len(png) {
		t.Fatal(r.Code, r.Body.String())
	}
	// Existing receipts keep their PNG URLs while disk storage becomes AVIF.
	avif := append([]byte{0, 0, 0, 24}, []byte("ftypavif0000000000000000")...)
	if err := os.WriteFile(filepath.Join(dir, "page.avif"), avif, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "page.png")); err != nil {
		t.Fatal(err)
	}
	if r := send("127.0.0.1:1234"); r.Code != 200 || r.Header().Get("Content-Type") != "image/avif" || len(r.Body.Bytes()) != len(avif) {
		t.Fatal(r.Code, r.Body.String())
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "page.png"), png, 0600)
	s.Store.DB.Exec("UPDATE source_captures SET artifact_dir=?", outside)
	if r := send("127.0.0.1:1234"); r.Code != 503 {
		t.Fatal("read outside capture storage", r.Code)
	}
}
