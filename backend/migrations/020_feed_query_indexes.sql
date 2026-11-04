-- Cover status counters and filtered case ordering without reading evidence/drafts.
CREATE INDEX cases_status_updated_at ON cases(status,updated_at);
-- Receipt-time sorting (with the implicit rowid as its pagination tie-breaker).
CREATE INDEX submissions_created_at ON submissions(created_at);
CREATE INDEX submissions_origin_created_at ON submissions(origin,created_at);
-- The older (case_id,created_at,id) index cannot serve ORDER BY rowid per case.
CREATE INDEX research_runs_case_sequence ON research_runs(case_id);
-- Restrict rolling/today statistics to the requested research-start window.
CREATE INDEX research_runs_started_at ON research_runs(started_at) WHERE started_at>0;
