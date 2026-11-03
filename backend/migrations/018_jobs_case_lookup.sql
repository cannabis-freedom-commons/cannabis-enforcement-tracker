-- The submissions feed and status counters repeatedly look up jobs by case,
-- including queries without a kind. UNIQUE(kind,ref) cannot serve that lookup.
CREATE INDEX IF NOT EXISTS jobs_ref_status ON jobs(ref,status,lease_until);
