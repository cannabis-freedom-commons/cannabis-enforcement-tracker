-- Reports are committed to a submissions branch and reach main in batches (worker/batch.go): each
-- batch is one push of the branch to main; its reports are published once the site is deployed.
CREATE TABLE publication_batches (
 id INTEGER PRIMARY KEY,
 merged_at INTEGER NOT NULL,
 commit_sha TEXT NOT NULL,
 reports INTEGER NOT NULL,
 deployed_at INTEGER NOT NULL DEFAULT 0
);
ALTER TABLE local_publications ADD COLUMN batch_id INTEGER REFERENCES publication_batches(id);
-- Until now every committed report was pushed to main at once, and the site has long been rebuilt
-- since: they are published.
INSERT INTO submission_history(submission_id,status,explanation,actor,created_at)
 SELECT s.id,'published','Published on the site.','migration',strftime('%s','now')
 FROM submissions s JOIN cases c ON c.id=s.case_id WHERE c.status='committed';
UPDATE cases SET status='published' WHERE status='committed';
