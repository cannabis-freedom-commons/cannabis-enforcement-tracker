package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A deployed backend lets the operator's token restart (the site's dev proxy sends it), nobody else.
func TestRestartWithOperatorToken(t *testing.T) {
	s, _ := server(t)
	s.Config.Addr = "0.0.0.0:8080" // deployed: no localhost controls
	s.Config.OperatorToken = strings.Repeat("k", 32)
	post := func(auth string) int {
		r := httptest.NewRequest("POST", "/v1/submissions/"+strings.Repeat("0", 32)+"/restart", strings.NewReader(`{}`))
		r.RemoteAddr = "203.0.113.9:1234"
		r.Header.Set("Content-Type", "application/json")
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if code := post(""); code != 403 {
		t.Fatal("restart without the token", code)
	}
	if code := post(strings.Repeat("x", 32)); code != 403 {
		t.Fatal("restart with a wrong token", code)
	}
	if code := post(strings.Repeat("k", 32)); code == 403 {
		t.Fatal("the operator's token was refused")
	}
	r := httptest.NewRequest("GET", "/v1/config", nil)
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("k", 32))
	if !s.processingInfo(r).RestartAvailable || s.processingInfo(httptest.NewRequest("GET", "/v1/config", nil)).RestartAvailable {
		t.Fatal("restart availability does not follow the token")
	}
}
