package api

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func (s *Server) sourceScreenshot(w http.ResponseWriter, r *http.Request) {
	s.sourceFile(w, r, "page")
}

// sourceFile serves a page capture of a submission's source (its screenshot) to the request page:
// public, like the rest of the request, but only to the tracker's own pages (no hotlinking from
// other sites), and only as a validated image that cannot run anything.
func (s *Server) sourceFile(w http.ResponseWriter, r *http.Request, name string) {
	if !s.ownPage(w, r) {
		return
	}
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		problem(w, 404, "Receipt not found.")
		return
	}
	var dir string
	// the capture the source check names (also one of an archived copy of the page), else the
	// newest capture of the submitted page itself
	query := `SELECT p.artifact_dir FROM source_captures p JOIN submissions s ON s.case_id=p.case_id JOIN cases c ON c.id=p.case_id WHERE s.id=? AND p.url=c.canonical_url`
	args := []any{id}
	if capture := r.URL.Query().Get("capture"); capture != "" {
		if !idPattern.MatchString(capture) {
			problem(w, 404, "Capture not found.")
			return
		}
		query = `SELECT p.artifact_dir FROM source_captures p JOIN submissions s ON s.case_id=p.case_id WHERE s.id=? AND p.id=?`
		args = append(args, capture)
	}
	err := s.Store.Reader().QueryRowContext(r.Context(), query+" ORDER BY p.created_at DESC,p.rowid DESC LIMIT 1", args...).Scan(&dir)
	if err != nil {
		if store.IsMissingAI(err) {
			problem(w, 404, "No captured screenshot is available.")
		} else {
			problem(w, 503, "Screenshot unavailable.")
		}
		return
	}
	s.serveCapture(w, r, dir, name)
}

// ownPage lets a screenshot go only to the tracker's own pages (no hotlinking from other sites).
func (s *Server) ownPage(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		problem(w, 403, "Source screenshots are shown on the tracker's own pages.")
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		allowed := e == nil && loopbackHost(u.Hostname())
		for _, o := range s.Config.Origins {
			allowed = allowed || origin == o
		}
		if !allowed {
			problem(w, 403, "origin not allowed")
			return false
		}
	}
	return true
}

// sourceByURL serves the newest screenshot the research took of a page, found by its address: what a
// report's source looked like when it was read (the source tooltip on a report page). Only pages
// the tracker captured itself; an address it never opened has none.
func (s *Server) sourceByURL(w http.ResponseWriter, r *http.Request) {
	if !s.ownPage(w, r) {
		return
	}
	raw := r.URL.Query().Get("url")
	if raw == "" || len(raw) > 4000 {
		problem(w, 404, "Capture not found.")
		return
	}
	candidates := []any{raw}
	if normalized, err := safeurl.Normalize(raw); err == nil && normalized != raw {
		candidates = append(candidates, normalized)
	}
	var dir string
	err := s.Store.Reader().QueryRowContext(r.Context(), `SELECT artifact_dir FROM source_captures WHERE url IN (?`+strings.Repeat(",?", len(candidates)-1)+`) ORDER BY created_at DESC,rowid DESC LIMIT 1`, candidates...).Scan(&dir)
	if err != nil {
		if store.IsMissingAI(err) {
			problem(w, 404, "No captured screenshot is available.")
		} else {
			problem(w, 503, "Screenshot unavailable.")
		}
		return
	}
	s.serveCapture(w, r, dir, "page")
}

// serveCapture sends one image of a capture's folder inside the research workspaces, checked to be a
// real image that cannot run anything.
func (s *Server) serveCapture(w http.ResponseWriter, r *http.Request, dir, name string) {
	base, err := filepath.Abs(s.Config.Workspaces)
	if err != nil {
		problem(w, 503, "Screenshot unavailable.")
		return
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		problem(w, 503, "Screenshot unavailable.")
		return
	}
	relative, err := filepath.Rel(base, dir)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		problem(w, 503, "Invalid capture storage.")
		return
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		problem(w, 404, "Screenshot unavailable.")
		return
	}
	defer root.Close()
	var file *os.File
	for _, ext := range []string{".webp", ".png", ".avif"} {
		file, err = root.Open(filepath.Join(relative, filepath.FromSlash(name+ext)))
		if !os.IsNotExist(err) {
			name += ext
			break
		}
	}
	if err != nil {
		problem(w, 404, "Screenshot unavailable.")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64*1024*1024 { // a whole long page at full width
		problem(w, 503, "Invalid screenshot.")
		return
	}
	header := make([]byte, 32)
	n, readErr := file.Read(header)
	contentType := ""
	if n >= 8 && bytes.Equal(header[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		contentType = "image/png"
	}
	if n >= 12 && string(header[4:8]) == "ftyp" && (string(header[8:12]) == "avif" || string(header[8:12]) == "avis") {
		contentType = "image/avif"
	}
	if n >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP" {
		contentType = "image/webp"
	}
	if readErr != nil || contentType == "" {
		problem(w, 503, "Invalid screenshot.")
		return
	}
	file.Seek(0, 0)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-site")
	w.Header().Set("Content-Disposition", `inline; filename="source-`+strings.ReplaceAll(name, "/", "-")+`"`)
	w.Header().Set("Cache-Control", "private, max-age=3600") // a capture never changes once taken
	http.ServeContent(w, r, "source-"+strings.ReplaceAll(name, "/", "-"), info.ModTime(), file)
}
