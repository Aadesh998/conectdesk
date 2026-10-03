-- name: get-assistants
SELECT a.id, a.created_at, a.updated_at, a.user_id, u.first_name AS name, u.avatar_url,
       a.description, a.instructions, a.guardrails, a.expectation, a.tone, a.response_length, a.max_turns, a.fallback_team_id, a.handoff_enabled, a.languages, a.enabled
FROM ai_assistants a
JOIN users u ON u.id = a.user_id AND u.deleted_at IS NULL
WHERE a.tenant_id = $1
ORDER BY a.updated_at DESC;

-- name: get-assistant
SELECT a.id, a.created_at, a.updated_at, a.user_id, u.first_name AS name, u.avatar_url,
       a.description, a.instructions, a.guardrails, a.expectation, a.tone, a.response_length, a.max_turns, a.fallback_team_id, a.handoff_enabled, a.languages, a.enabled
FROM ai_assistants a
JOIN users u ON u.id = a.user_id AND u.deleted_at IS NULL
WHERE a.tenant_id = $1 AND a.id = $2;

-- name: get-assistant-by-user-id
SELECT a.id, a.created_at, a.updated_at, a.user_id, u.first_name AS name, u.avatar_url,
       a.description, a.instructions, a.guardrails, a.expectation, a.tone, a.response_length, a.max_turns, a.fallback_team_id, a.handoff_enabled, a.languages, a.enabled
FROM ai_assistants a
JOIN users u ON u.id = a.user_id AND u.deleted_at IS NULL
WHERE a.tenant_id = $1 AND a.user_id = $2;

-- name: get-assistant-user-ids
-- tenant: all, user IDs are unique across tenants, so one set recognizes AI-authored replies everywhere.
-- Deleted assistants stay included: their old replies must still be recognized as AI-authored.
SELECT id FROM users WHERE type = 'ai_assistant';

-- name: insert-assistant-user
INSERT INTO users (tenant_id, type, first_name, last_name, enabled)
VALUES ($1, 'ai_assistant', $2, '', true)
RETURNING id;

-- name: insert-assistant
INSERT INTO ai_assistants (tenant_id, user_id, description, instructions, guardrails, tone, response_length, max_turns, fallback_team_id, enabled, expectation, handoff_enabled, languages)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id;

-- name: update-assistant
UPDATE ai_assistants
SET description = $3, instructions = $4, guardrails = $5, tone = $6, response_length = $7, max_turns = $8, fallback_team_id = $9, enabled = $10, expectation = $11, handoff_enabled = $12, languages = $13, updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: get-assistant-expectation-by-user-id
SELECT expectation FROM ai_assistants WHERE tenant_id = $1 AND user_id = $2;

-- name: update-assistant-user
UPDATE users SET first_name = $3, updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND type = 'ai_assistant';

-- name: soft-delete-assistant-user
UPDATE users SET deleted_at = now(), updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND type = 'ai_assistant';

-- name: delete-assistant
DELETE FROM ai_assistants WHERE tenant_id = $1 AND id = $2;

-- name: unassign-assistant-conversations
UPDATE conversations
SET assigned_user_id = NULL, assigned_team_id = COALESCE($3, assigned_team_id), updated_at = now()
WHERE tenant_id = $1 AND assigned_user_id = $2
  AND status_id IN (SELECT id FROM conversation_statuses WHERE tenant_id = $1 AND category <> 'resolved');

-- name: get-assistant-tools
SELECT tool_id FROM ai_assistant_tools WHERE tenant_id = $1 AND assistant_id = $2 ORDER BY tool_id;

-- name: get-all-assistant-tools
SELECT assistant_id, tool_id FROM ai_assistant_tools WHERE tenant_id = $1 ORDER BY assistant_id, tool_id;

-- name: delete-assistant-tools
DELETE FROM ai_assistant_tools WHERE tenant_id = $1 AND assistant_id = $2;

-- name: insert-assistant-tool
INSERT INTO ai_assistant_tools (tenant_id, assistant_id, tool_id)
VALUES ($1, $2, $3)
ON CONFLICT (assistant_id, tool_id) DO NOTHING;

-- name: insert-ai-agent-event
INSERT INTO ai_agent_events (tenant_id, assistant_id, conversation_id, type) VALUES ($1, $2, $3, $4);

-- name: get-assistant-window-stats
-- $1 = tenant, $2 = assistant user id (message sender), $3 = assistant id (events), $4 = window start, $5 = window end.
SELECT
  (SELECT count(DISTINCT conversation_id) FROM conversation_messages WHERE tenant_id = $1 AND sender_id = $2 AND type = 'outgoing' AND private = false AND created_at >= $4 AND created_at < $5 AND NOT COALESCE((meta->>'is_csat')::boolean, false)) AS conversations,
  (SELECT count(*) FROM conversation_messages WHERE tenant_id = $1 AND sender_id = $2 AND type = 'outgoing' AND private = false AND created_at >= $4 AND created_at < $5 AND NOT COALESCE((meta->>'is_csat')::boolean, false) AND NOT COALESCE((meta->>'is_confirmation')::boolean, false)) AS replies,
  (SELECT count(DISTINCT conversation_id) FROM ai_agent_events WHERE tenant_id = $1 AND assistant_id = $3 AND type = 'handoff' AND created_at >= $4 AND created_at < $5) AS handoffs,
  (SELECT count(DISTINCT conversation_id) FROM ai_agent_events WHERE tenant_id = $1 AND assistant_id = $3 AND type = 'resolve' AND created_at >= $4 AND created_at < $5) AS resolves,
  (SELECT count(DISTINCT e.conversation_id) FROM ai_agent_events e JOIN conversations c ON c.id = e.conversation_id JOIN conversation_statuses s ON s.id = c.status_id
     WHERE e.tenant_id = $1 AND e.assistant_id = $3 AND e.type = 'resolve' AND e.created_at >= $4 AND e.created_at < $5 AND s.category <> 'resolved') AS reopened,
  (SELECT count(*) FROM csat_responses cr WHERE cr.tenant_id = $1 AND cr.rating > 0 AND cr.created_at >= $4 AND cr.created_at < $5 AND EXISTS (
     SELECT 1 FROM conversation_messages m WHERE m.conversation_id = cr.conversation_id AND m.sender_id = $2 AND m.type = 'outgoing' AND m.private = false AND NOT COALESCE((m.meta->>'is_csat')::boolean, false))) AS csat_count,
  COALESCE((SELECT round(avg(cr.rating)::numeric, 2) FROM csat_responses cr WHERE cr.tenant_id = $1 AND cr.rating > 0 AND cr.created_at >= $4 AND cr.created_at < $5 AND EXISTS (
     SELECT 1 FROM conversation_messages m WHERE m.conversation_id = cr.conversation_id AND m.sender_id = $2 AND m.type = 'outgoing' AND m.private = false AND NOT COALESCE((m.meta->>'is_csat')::boolean, false))), 0)::float8 AS csat_avg,
  COALESCE((SELECT round((count(*) FILTER (WHERE cr.rating >= 4))::numeric / NULLIF(count(*), 0) * 100, 1) FROM csat_responses cr WHERE cr.tenant_id = $1 AND cr.rating > 0 AND cr.created_at >= $4 AND cr.created_at < $5 AND EXISTS (
     SELECT 1 FROM conversation_messages m WHERE m.conversation_id = cr.conversation_id AND m.sender_id = $2 AND m.type = 'outgoing' AND m.private = false AND NOT COALESCE((m.meta->>'is_csat')::boolean, false))), 0)::float8 AS csat_positive;

-- name: count-ai-turns-since-assignment
-- Counts the assistant's public non-CSAT replies since the last assignment activity.
SELECT count(*) FROM conversation_messages
WHERE tenant_id = $1 AND conversation_id = $2 AND sender_id = $3 AND type = 'outgoing' AND private = false
  AND NOT COALESCE((meta->>'is_csat')::boolean, false)
  AND NOT COALESCE((meta->>'is_confirmation')::boolean, false)
  AND created_at > COALESCE((
    SELECT max(created_at) FROM conversation_messages
    WHERE tenant_id = $1 AND conversation_id = $2 AND type = 'activity'
      AND meta->>'activity_type' IN ('assigned_user_change', 'self_assign')
  ), to_timestamp(0));

-- name: get-recent-contact-conversations
SELECT c.uuid, c.reference_number, c.created_at, COALESCE(c.subject, '') AS subject, s.name AS status
FROM conversations c
JOIN conversation_statuses s ON s.id = c.status_id
WHERE c.tenant_id = $1 AND c.contact_id = $2 AND c.id != $3 AND c.created_at >= now() - make_interval(days => $4)
ORDER BY c.created_at DESC
LIMIT $5;

-- name: insert-faq-suggestion
INSERT INTO ai_faq_suggestions (tenant_id, conversation_id, question, answer) VALUES ($1, $2, $3, $4);

-- name: count-faq-suggestions-by-conversation
SELECT count(*) FROM ai_faq_suggestions WHERE tenant_id = $1 AND conversation_id = $2;

-- name: pending-faq-question-exists
SELECT EXISTS(SELECT 1 FROM ai_faq_suggestions WHERE tenant_id = $1 AND status = 'pending' AND lower(question) = lower($2));

-- name: get-faq-suggestions
SELECT s.id, s.created_at, s.updated_at, s.conversation_id, s.question, s.answer, s.status, s.reviewed_by_id, s.reviewed_at,
       c.uuid AS conversation_uuid, c.reference_number
FROM ai_faq_suggestions s
JOIN conversations c ON c.id = s.conversation_id
WHERE s.tenant_id = $1 AND ($2 = '' OR s.status = $2)
ORDER BY s.created_at DESC;

-- name: get-faq-suggestion
SELECT id, created_at, updated_at, conversation_id, question, answer, status, reviewed_by_id, reviewed_at
FROM ai_faq_suggestions WHERE tenant_id = $1 AND id = $2;

-- name: reject-faq-suggestion-if-pending
UPDATE ai_faq_suggestions SET status = 'rejected', reviewed_by_id = $3, reviewed_at = now(), updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND status = 'pending';

-- name: revert-faq-suggestion-to-pending
UPDATE ai_faq_suggestions SET status = 'pending', reviewed_by_id = NULL, reviewed_at = NULL, updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND status = 'approved';

-- name: approve-faq-suggestion-if-pending
UPDATE ai_faq_suggestions SET status = 'approved', reviewed_by_id = $3, reviewed_at = now(), updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND status = 'pending';
