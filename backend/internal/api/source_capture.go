package api

import (
	"bytes"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func (s *Server) sourceScreenshot(w http.ResponseWriter, r *http.Request) {
	s.sourceFile(w, r, "page.png")
}

// sourceImage serves one of the article's photos the capture kept (images/<n>.png), local only
// like the screenshot.
func (s *Server) sourceImage(w http.ResponseWriter, r *http.Request) {
	n := r.PathValue("n")
	if len(n) != 1 || n < "1" || n > "8" {
		problem(w, 404, "Image not found.")
		return
	}
	s.sourceFile(w, r, "images/"+n+".png")
}

func (s *Server) sourceFile(w http.ResponseWriter, r *http.Request, name string) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if !s.localControls() || err != nil || !loopbackHost(host) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		problem(w, 403, "Source screenshots are available only on the local development backend.")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || !loopbackHost(u.Hostname()) {
			problem(w, 403, "A localhost origin is required.")
			return
		}
	}
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		problem(w, 404, "Receipt not found.")
		return
	}
	var dir string
	query := `SELECT p.artifact_dir FROM source_captures p JOIN submissions s ON s.case_id=p.case_id JOIN cases c ON c.id=p.case_id WHERE s.id=? AND p.url=c.canonical_url`
	args := []any{id}
	if capture := r.URL.Query().Get("capture"); capture != "" {
		if !idPattern.MatchString(capture) {
			problem(w, 404, "Capture not found.")
			return
		}
		query += " AND p.id=?"
		args = append(args, capture)
	}
	err = s.Store.DB.QueryRowContext(r.Context(), query+" ORDER BY p.created_at DESC,p.rowid DESC LIMIT 1", args...).Scan(&dir)
	if err != nil {
		if store.IsMissingAI(err) {
			problem(w, 404, "No captured screenshot is available.")
		} else {
			problem(w, 503, "Screenshot unavailable.")
		}
		return
	}
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
	file, err := root.Open(filepath.Join(relative, filepath.FromSlash(name)))
	if os.IsNotExist(err) {
		name = strings.TrimSuffix(name, ".png") + ".avif"
		file, err = root.Open(filepath.Join(relative, filepath.FromSlash(name)))
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
	if readErr != nil || contentType == "" {
		problem(w, 503, "Invalid screenshot.")
		return
	}
	file.Seek(0, 0)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `inline; filename="source-`+strings.ReplaceAll(name, "/", "-")+`"`)
	w.Header().Set("Cache-Control", "private, max-age=3600") // a capture never changes once taken
	http.ServeContent(w, r, "source-"+strings.ReplaceAll(name, "/", "-"), info.ModTime(), file)
}
