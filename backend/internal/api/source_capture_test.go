package api

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func TestSourceScreenshotIsPublicAndBoundToReceipt(t *testing.T) {
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
	request := func(remote, query string, header ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/v1/submissions/"+id+"/source/screenshot"+query, nil)
		r.RemoteAddr = remote
		for i := 0; i+1 < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	send := func(remote string) *httptest.ResponseRecorder { return request(remote, "") }
	// the request page on the public site shows it, from anywhere
	if r := request("198.51.100.1:1234", "", "Sec-Fetch-Site", "same-site"); r.Code != 200 || r.Header().Get("Content-Type") != "image/png" || r.Header().Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
		t.Fatal("public screenshot not served", r.Code, r.Body.String())
	}
	// other sites may not embed it
	if r := request("198.51.100.1:1234", "", "Sec-Fetch-Site", "cross-site"); r.Code != 403 {
		t.Fatal("hotlinked from another site", r.Code)
	}
	if r := request("198.51.100.1:1234", "", "Origin", "https://elsewhere.example"); r.Code != 403 {
		t.Fatal("foreign origin allowed", r.Code)
	}
	// a capture of an archived copy of the page is found by its id, a capture of another case is not
	archive := store.ID()
	archiveDir := filepath.Join(s.Config.Workspaces, "sources", v.CaseID, archive)
	os.MkdirAll(archiveDir, 0700)
	os.WriteFile(filepath.Join(archiveDir, "page.png"), png, 0600)
	s.Store.DB.Exec("INSERT INTO source_captures(id,case_id,url,artifact_dir,created_at) VALUES(?,?,?,?,?)", archive, v.CaseID, "https://web.archive.org/web/2025/https://news.example/screenshot", archiveDir, time.Now().Unix())
	if r := request("198.51.100.1:1234", "?capture="+archive); r.Code != 200 {
		t.Fatal("archived capture not served", r.Code, r.Body.String())
	}
	other, _ := s.Store.Accept(context.Background(), store.Intake{Origin: "web", URL: "https://news.example/other", CanonicalURL: "https://news.example/other"})
	o, _ := s.Store.Get(context.Background(), other)
	foreign := store.ID()
	s.Store.DB.Exec("INSERT INTO source_captures(id,case_id,url,artifact_dir,created_at) VALUES(?,?,?,?,?)", foreign, o.CaseID, o.URL, archiveDir, time.Now().Unix())
	if r := request("198.51.100.1:1234", "?capture="+foreign); r.Code != 404 {
		t.Fatal("another case's capture served", r.Code)
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
	webp := []byte("RIFF\x04\x00\x00\x00WEBP")
	if err := os.WriteFile(filepath.Join(dir, "page.webp"), webp, 0600); err != nil {
		t.Fatal(err)
	}
	if r := send("127.0.0.1:1234"); r.Code != 200 || r.Header().Get("Content-Type") != "image/webp" || len(r.Body.Bytes()) != len(webp) {
		t.Fatal(r.Code, r.Body.String())
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "page.png"), png, 0600)
	s.Store.DB.Exec("UPDATE source_captures SET artifact_dir=?", outside)
	if r := send("127.0.0.1:1234"); r.Code != 503 {
		t.Fatal("read outside capture storage", r.Code)
	}
}

// A report's source tooltip finds the screenshot of a page by its address, only from the tracker's pages.
func TestSourceScreenshotByURL(t *testing.T) {
	s, _ := server(t)
	s.Config.Workspaces = t.TempDir()
	id, err := s.Store.Accept(context.Background(), store.Intake{Origin: "web", URL: "https://news.example/story", CanonicalURL: "https://news.example/story"})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := s.Store.Get(context.Background(), id)
	dir := filepath.Join(s.Config.Workspaces, "sources", v.CaseID, store.ID())
	os.MkdirAll(dir, 0700)
	if err = os.WriteFile(filepath.Join(dir, "page.png"), []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0}, 0600); err != nil {
		t.Fatal(err)
	}
	// another source the research opened for the same case
	if _, err = s.Store.DB.Exec("INSERT INTO source_captures(id,case_id,url,artifact_dir,created_at) VALUES(?,?,?,?,?)", store.ID(), v.CaseID, "https://other.example/account", dir, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	get := func(u string, header ...string) int {
		r := httptest.NewRequest("GET", "/v1/sources/screenshot?url="+url.QueryEscape(u), nil)
		for i := 0; i+1 < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if code := get("https://other.example/account", "Sec-Fetch-Site", "same-site"); code != 200 {
		t.Fatal("captured source not served", code)
	}
	if code := get("https://never.example/opened"); code != 404 {
		t.Fatal("a page never captured", code)
	}
	if code := get("https://other.example/account", "Sec-Fetch-Site", "cross-site"); code != 403 {
		t.Fatal("hotlinked", code)
	}
}
