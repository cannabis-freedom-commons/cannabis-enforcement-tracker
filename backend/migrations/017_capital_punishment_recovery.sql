-- Re-research capital-punishment referrals under the corrected scope policy.
-- Other CFI referrals and active jobs stay untouched; research checks all routing rules again.
CREATE TEMP TABLE capital_punishment_recovery AS
 SELECT id FROM cases WHERE status='routed_to_cfi'
 AND (lower(explanation) LIKE '%death sentence%' OR lower(explanation) LIKE '%execution%')
 AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.ref=cases.id AND j.status='running');
DELETE FROM research_checkpoints WHERE case_id IN (SELECT id FROM capital_punishment_recovery);
DELETE FROM jobs WHERE kind='publish' AND ref IN (SELECT id FROM capital_punishment_recovery);
INSERT INTO jobs(id,kind,ref,payload,status,available_at)
 SELECT lower(hex(randomblob(16))),'research',id,'{}','queued',unixepoch() FROM capital_punishment_recovery WHERE 1
 ON CONFLICT(kind,ref) DO UPDATE SET status='queued',attempts=0,available_at=excluded.available_at,lease_until=0,lease_token='',error_code='';
INSERT INTO submission_history(submission_id,status,explanation,actor,created_at)
 SELECT id,'queued','Cannabis-related death sentences and executions belong in the tracker. Queued for fresh research under the corrected scope policy.','recovery',unixepoch()
 FROM submissions WHERE case_id IN (SELECT id FROM capital_punishment_recovery);
UPDATE cases SET status='queued',explanation='Cannabis-related death sentences and executions belong in the tracker. Queued for fresh research under the corrected scope policy.',
 source_check='',ai_result='',validation='',draft_path='',draft_markdown='',report_slug='',pr_url='',updated_at=unixepoch()
 WHERE id IN (SELECT id FROM capital_punishment_recovery);
DELETE FROM outbox WHERE kind='resolution' AND status<>'done'
 AND ref IN (SELECT id||':routed_to_cfi' FROM submissions WHERE case_id IN (SELECT id FROM capital_punishment_recovery));
DROP TABLE capital_punishment_recovery;
