-- name: summary
-- A workspace's day between $2 and $3: new conversations, contacts who got in
-- touch for the first time, calls in and missed, call-backs done, those still
-- waiting and overdue now, and admissions (moves onto the stages in $4).
SELECT
	(SELECT COUNT(*) FROM conversations
		WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3) AS enquiries,
	(SELECT COUNT(DISTINCT contact_id) FROM lead_stage_events
		WHERE tenant_id = $1 AND from_stage IS NULL AND created_at >= $2 AND created_at < $3) AS new_leads,
	(SELECT COUNT(*) FROM conversation_messages
		WHERE tenant_id = $1 AND meta ? 'call' AND meta->'call'->>'direction' = 'inbound'
		AND created_at >= $2 AND created_at < $3) AS calls_in,
	(SELECT COUNT(*) FROM conversation_messages
		WHERE tenant_id = $1 AND meta ? 'call' AND meta->'call'->>'direction' = 'inbound'
		AND meta->'call'->>'status' IN ('no-answer', 'busy', 'failed', 'voicemail')
		AND created_at >= $2 AND created_at < $3) AS missed_calls,
	(SELECT COUNT(*) FROM call_backs
		WHERE tenant_id = $1 AND status = 'done' AND done_at >= $2 AND done_at < $3) AS called_back,
	(SELECT COUNT(*) FROM call_backs WHERE tenant_id = $1 AND status = 'pending') AS pending_call_backs,
	(SELECT COUNT(*) FROM call_backs WHERE tenant_id = $1 AND status = 'pending' AND due_at < NOW()) AS overdue_call_backs,
	(SELECT COUNT(DISTINCT contact_id) FROM lead_stage_events
		WHERE tenant_id = $1 AND to_stage = ANY($4::TEXT[]) AND created_at >= $2 AND created_at < $3) AS admissions;

-- name: enabled-tenants
SELECT tenant_id FROM settings WHERE key = 'daily_report.enabled' AND value = 'true'::jsonb;

-- name: claim-run
-- Claims a workspace's report for a day, once however many instances try.
INSERT INTO daily_report_runs (tenant_id, report_date) VALUES ($1, $2)
ON CONFLICT (tenant_id, report_date) DO NOTHING
RETURNING id;

-- name: finish-run
UPDATE daily_report_runs SET status = $3, error = $4 WHERE tenant_id = $1 AND id = $2;

-- name: last-runs
SELECT report_date::TEXT AS report_date, created_at, status, error FROM daily_report_runs
WHERE tenant_id = $1 ORDER BY report_date DESC LIMIT 7;
