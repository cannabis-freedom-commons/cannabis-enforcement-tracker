package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

func loopbackHost(host string) bool {
	return host == "localhost" || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}
func (s *Server) localControls() bool {
	host, _, err := net.SplitHostPort(s.Config.Addr)
	return s.Config.DevControls && err == nil && loopbackHost(host)
}

// Local development intake can bypass rate limits, including through Astro's
// loopback proxy. A client-supplied forwarding header cannot grant this access.
func (s *Server) localDevelopmentRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if !s.localControls() || err != nil || !loopbackHost(host) {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !loopbackHost(u.Hostname()) {
			return false
		}
	}
	return true
}

// operator is a request carrying OPERATOR_TOKEN: the site's local dev proxy on the operator's own
// machine, talking to a deployed backend.
func (s *Server) operator(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && s.Config.OperatorToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.Config.OperatorToken)) == 1
}

// restartAllowed: the local development backend asked from this machine, or the operator's token.
func (s *Server) restartAllowed(r *http.Request) bool {
	if s.operator(r) {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return s.localControls() && err == nil && loopbackHost(host)
}

func (s *Server) restart(w http.ResponseWriter, r *http.Request) {
	if !s.restartAllowed(r) {
		problem(w, 403, "Restart is available on the local development backend, or with the operator token.")
		return
	}
	// Require a non-simple JSON request and, without the token, a local browser origin, including
	// when Astro proxies the request. Never use forwarded IP headers for this control.
	content, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if content != "application/json" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		problem(w, 403, "Use the localhost submission restart control.")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && !s.operator(r) {
		u, e := url.Parse(origin)
		if e != nil || !loopbackHost(u.Hostname()) {
			problem(w, 403, "Restart requires a localhost origin.")
			return
		}
	}
	var err error
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		problem(w, 404, "Receipt not found.")
		return
	}
	// {"resume": true} retries after the saved AI research; otherwise everything starts again
	var body struct {
		Resume bool `json:"resume"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body)
	if body.Resume {
		err = s.Store.Retry(r.Context(), id)
	} else {
		err = s.Store.Restart(r.Context(), id)
	}
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNothingToResume):
			problem(w, 409, "This submission has no finished research to resume from. Restart it instead.")
		case store.IsMissingAI(err):
			problem(w, 404, "Receipt not found.")
		case errors.Is(err, store.ErrRestartConflict):
			problem(w, 409, "This submission is already running or is not failed or budget paused.")
		default:
			problem(w, 503, "Submission could not be restarted.")
		}
		return
	}
	write(w, 202, map[string]string{"id": id, "status": "queued"})
}
