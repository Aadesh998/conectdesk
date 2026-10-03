-- name: get-sla-policy
SELECT id, name, description, first_response_time, resolution_time, next_response_time, notifications, created_at, updated_at FROM sla_policies WHERE tenant_id = $1 AND id = $2;

-- name: get-all-sla-policies
SELECT id, name, description, first_response_time, resolution_time, next_response_time, notifications, created_at, updated_at FROM sla_policies WHERE tenant_id = $1 ORDER BY updated_at DESC;

-- name: insert-sla-policy
INSERT INTO sla_policies (
   tenant_id,
   name,
   description, 
   first_response_time,
   resolution_time,
   next_response_time,
   notifications
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: update-sla-policy
UPDATE sla_policies SET
   name = $3,
   description = $4,
   first_response_time = $5,
   resolution_time = $6,
   next_response_time = $7,
   notifications = $8,
   updated_at = NOW()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: delete-sla-policy
DELETE FROM sla_policies WHERE tenant_id = $1 AND id = $2;

-- name: apply-sla
WITH deleted AS (
  DELETE FROM applied_slas WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'pending'
),
new_sla AS (
  INSERT INTO applied_slas (
    tenant_id,
    conversation_id,
    sla_policy_id,
    first_response_deadline_at,
    resolution_deadline_at
  ) VALUES ($1, $2, $3, $4, $5)
  RETURNING conversation_id, id
)
-- update the conversation with the new SLA policy and next SLA deadline.
UPDATE conversations c
SET
   sla_policy_id = $3,
   next_sla_deadline_at = LEAST($4, $5)
FROM new_sla ns
WHERE c.tenant_id = $1 AND c.id = ns.conversation_id
RETURNING ns.id;

-- name: get-pending-applied-sla
-- tenant: all, the SLA evaluator covers every tenant; each row carries its tenant_id.
-- Returns only actionable pending SLAs: a metric is unresolved AND either its deadline has passed
-- or the conversation has transitioned (first reply / resolve) since the last evaluation.
SELECT a.id, a.tenant_id, a.first_response_deadline_at, c.first_reply_at as conversation_first_response_at, a.sla_policy_id,
a.resolution_deadline_at, c.resolved_at as conversation_resolved_at, c.id as conversation_id, a.first_response_met_at, a.resolution_met_at, a.first_response_breached_at, a.resolution_breached_at
FROM applied_slas a
JOIN conversations c ON a.conversation_id = c.id and c.sla_policy_id = a.sla_policy_id
WHERE a.status = 'pending'::applied_sla_status
  AND (
    (a.first_response_met_at IS NULL AND a.first_response_breached_at IS NULL
     AND (a.first_response_deadline_at <= NOW() OR c.first_reply_at IS NOT NULL))
    OR
    (a.resolution_met_at IS NULL AND a.resolution_breached_at IS NULL
     AND (a.resolution_deadline_at <= NOW() OR c.resolved_at IS NOT NULL))
  );

-- name: update-applied-sla-breached-at
UPDATE applied_slas SET
   first_response_breached_at = CASE WHEN $3 = 'first_response' THEN NOW() ELSE first_response_breached_at END,
   resolution_breached_at = CASE WHEN $3 = 'resolution' THEN NOW() ELSE resolution_breached_at END,
   updated_at = NOW()
WHERE tenant_id = $1 AND id = $2;

-- name: update-applied-sla-met-at
UPDATE applied_slas SET
   first_response_met_at = CASE WHEN $3 = 'first_response' THEN NOW() ELSE first_response_met_at END,
   resolution_met_at = CASE WHEN $3 = 'resolution' THEN NOW() ELSE resolution_met_at END,
   updated_at = NOW()
WHERE tenant_id = $1 AND id = $2;

-- name: update-conversation-sla-deadline
UPDATE conversations c
SET next_sla_deadline_at = CASE
    -- If the conversation is in a resolved-category status, clear the deadline
    WHEN c.status_id IN (SELECT id FROM conversation_statuses WHERE tenant_id = $1 AND category = 'resolved') THEN NULL

    -- If an external timestamp ($3) is provided (e.g. next_response), use the earliest of $3.
    WHEN $3::TIMESTAMPTZ IS NOT NULL THEN LEAST(
        $3::TIMESTAMPTZ,
        CASE
            WHEN c.first_reply_at IS NOT NULL AND c.resolved_at IS NULL AND a.resolution_deadline_at IS NOT NULL THEN a.resolution_deadline_at
            WHEN c.first_reply_at IS NULL AND c.resolved_at IS NULL AND a.first_response_deadline_at IS NOT NULL THEN a.first_response_deadline_at
            WHEN a.first_response_deadline_at IS NOT NULL AND a.resolution_deadline_at IS NOT NULL THEN LEAST(a.first_response_deadline_at, a.resolution_deadline_at)
            ELSE NULL
        END
    )

    -- No $3,
    ELSE CASE
        WHEN c.first_reply_at IS NOT NULL AND c.resolved_at IS NULL AND a.resolution_deadline_at IS NOT NULL THEN a.resolution_deadline_at
        WHEN c.first_reply_at IS NULL AND c.resolved_at IS NULL AND a.first_response_deadline_at IS NOT NULL THEN a.first_response_deadline_at
        WHEN a.first_response_deadline_at IS NOT NULL AND a.resolution_deadline_at IS NOT NULL THEN LEAST(a.first_response_deadline_at, a.resolution_deadline_at)
        ELSE NULL
    END
END
FROM applied_slas a
WHERE a.conversation_id = c.id
AND c.tenant_id = $1
AND c.id = $2;

-- name: update-applied-sla-status
UPDATE applied_slas
SET
  status = CASE 
     WHEN first_response_met_at IS NOT NULL AND resolution_met_at IS NOT NULL THEN 'met'::applied_sla_status
     WHEN first_response_breached_at IS NOT NULL AND resolution_breached_at IS NOT NULL THEN 'breached'::applied_sla_status
     WHEN (first_response_met_at IS NOT NULL OR first_response_breached_at IS NOT NULL) 
          AND (resolution_met_at IS NOT NULL OR resolution_breached_at IS NOT NULL) THEN 'partially_met'::applied_sla_status
     WHEN first_response_met_at IS NULL AND first_response_breached_at IS NULL THEN 'pending'::applied_sla_status
     ELSE 'pending'::applied_sla_status
  END,
  updated_at = NOW()
WHERE applied_slas.tenant_id = $1 AND applied_slas.id = $2;

-- name: insert-scheduled-sla-notification
INSERT INTO scheduled_sla_notifications (
   tenant_id,
   applied_sla_id,
   sla_event_id,
   metric,
   notification_type,
   recipients,
   send_at
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: get-scheduled-sla-notifications
-- tenant: all, notifications of every tenant are sent; each row carries its tenant_id.
SELECT id, tenant_id, created_at, updated_at, applied_sla_id, sla_event_id, metric, notification_type, recipients, send_at, processed_at
FROM scheduled_sla_notifications
WHERE send_at <= NOW() AND processed_at IS NULL
ORDER BY send_at;

-- name: get-applied-sla
SELECT a.id,
   a.tenant_id,
   a.created_at,
   a.updated_at,
   a.conversation_id,
   a.sla_policy_id,
   a.first_response_deadline_at,
   a.resolution_deadline_at,
   a.first_response_met_at,
   a.resolution_met_at,
   a.first_response_breached_at,
   a.resolution_breached_at,
   a.status,
   c.first_reply_at as conversation_first_response_at,
   c.resolved_at as conversation_resolved_at,
   c.uuid as conversation_uuid,
   c.reference_number as conversation_reference_number,
   c.subject as conversation_subject,
   c.assigned_user_id as conversation_assigned_user_id,
   s.name as conversation_status,
   s.category as conversation_status_category
FROM applied_slas a INNER JOIN conversations c on a.conversation_id = c.id
LEFT JOIN conversation_statuses s ON c.status_id = s.id
WHERE a.tenant_id = $1 AND a.id = $2;

-- name: update-notification-processed
UPDATE scheduled_sla_notifications
SET processed_at = NOW(),
      updated_at = NOW()
WHERE tenant_id = $1 AND id = $2;

-- name: insert-next-response-sla-event
INSERT INTO sla_events (tenant_id, applied_sla_id, sla_policy_id, type, deadline_at)
SELECT $1, $2, $3, 'next_response', $4
WHERE NOT EXISTS (
  SELECT 1 FROM sla_events
  WHERE tenant_id = $1 AND applied_sla_id = $2 AND type = 'next_response' AND met_at IS NULL
)
RETURNING id;

-- name: set-latest-sla-event-met-at
UPDATE sla_events
SET met_at = NOW()
WHERE id = (
  SELECT id FROM sla_events
  WHERE tenant_id = $1 AND applied_sla_id = $2 AND type = $3 AND met_at IS NULL
  ORDER BY created_at DESC
  LIMIT 1
)
RETURNING met_at;

-- name: update-sla-event-as-breached
UPDATE sla_events
SET breached_at = NOW(),
    status = 'breached'
WHERE tenant_id = $1 AND id = $2;

-- name: update-sla-event-as-met
UPDATE sla_events
SET status = 'met'
WHERE tenant_id = $1 AND id = $2;

-- name: get-sla-event
SELECT id, tenant_id, created_at, updated_at, applied_sla_id, sla_policy_id, type, deadline_at, met_at, breached_at
FROM sla_events
WHERE tenant_id = $1 AND id = $2;

-- name: get-pending-sla-events
-- tenant: all, the SLA evaluator covers every tenant; each row carries its tenant_id.
-- Returns full event rows whose deadline has already passed (or that already have a met_at);
SELECT id, tenant_id, created_at, updated_at, applied_sla_id, sla_policy_id, type, deadline_at, met_at, breached_at
FROM sla_events
WHERE status = 'pending'
  AND deadline_at IS NOT NULL
  AND (deadline_at <= NOW() OR met_at IS NOT NULL);
