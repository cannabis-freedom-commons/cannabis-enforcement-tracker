-- How the agent got its starting context (files or inline), for comparing the two.
ALTER TABLE research_runs ADD COLUMN context_mode TEXT NOT NULL DEFAULT '';
