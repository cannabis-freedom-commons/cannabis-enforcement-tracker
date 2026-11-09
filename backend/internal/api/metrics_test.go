package api

import (
	"context"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
)

var sampleLine = regexp.MustCompile(`^[a-z_]+(\{([a-z_]+="([^"\\]|\\.)*",?)*\})? -?[0-9.e+-]+$`)

func TestPrometheusMetrics(t *testing.T) {
	a, h := server(t)
	ctx := context.Background()
	if _, err := a.Store.Accept(ctx, store.Intake{Origin: "web", URL: "https://news.example/a", CanonicalURL: "https://news.example/a"}); err != nil {
		t.Fatal(err)
	}
	a.Store.DB.Exec(`INSERT INTO research_runs(id,case_id,workspace,reserved_cents,created_at,model,status,input_tokens,output_tokens,cost_usd,started_at,finished_at,outcome)
		SELECT 'r1',id,'',100,strftime('%s','now'),'fixture/model "x"','completed',1200,300,0.25,100,160,'new_event' FROM cases`)
	a.WriteWorkerMetrics = func(p *Prom) {
		p.Metric("tracker_worker_up", "gauge", "test")
		p.Sample("tracker_worker_up", 1, "worker", "home")
	}
	handler := Instrument(h)
	if w := request(handler, "GET", "/metrics", "", ""); w.Code != 401 {
		t.Fatal("metrics without the token", w.Code)
	}
	r := httptest.NewRequest("GET", "/metrics", nil)
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		t.Fatal(w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`tracker_jobs{kind="research",status="queued"} 1`,
		`tracker_cases{status="queued"} 1`,
		`tracker_submissions_total{origin="web"} 1`,
		`tracker_research_tokens_total{model="fixture/model \"x\"",type="input"} 1200`,
		`tracker_research_cost_usd_total{model="fixture/model \"x\""} 0.25`,
		`tracker_research_run_seconds_sum{model="fixture/model \"x\""} 60`,
		`tracker_research_outcomes_total{outcome="new_event"} 1`,
		`tracker_budget_reserved_cents 100`,
		`tracker_worker_up{worker="home"} 1`,
		`tracker_http_requests_total{route="GET /metrics",code="4xx"} 1`,
	} {
		if !strings.Contains(body, want+"\n") {
			t.Errorf("missing %s", want)
		}
	}
	types := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if strings.HasPrefix(line, "# TYPE ") {
			name := strings.Fields(line)[2]
			if types[name] {
				t.Errorf("type declared twice: %s", name)
			}
			types[name] = true
			continue
		}
		if !strings.HasPrefix(line, "#") && !sampleLine.MatchString(line) {
			t.Errorf("not a Prometheus sample: %q", line)
		}
	}
	if t.Failed() {
		t.Log(body)
	}
}
