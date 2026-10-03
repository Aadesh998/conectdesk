-- name: get-overview-counts
SELECT
    json_build_object(
        'open',
        COUNT(*),
        'awaiting_response',
        COUNT(
            CASE
                WHEN c.last_message_sender = 'contact' THEN 1
            END
        ),
        'unassigned',
        COUNT(
            CASE
                WHEN c.assigned_user_id IS NULL THEN 1
            END
        ),
        'pending',
        COUNT(
            CASE
                WHEN c.first_reply_at IS NULL THEN 1
            END
        ),
        'agents_online',
        (
            SELECT
                COUNT(*)
            FROM
                users
            WHERE
                tenant_id = $1
                AND availability_status = 'online'
                AND type = 'agent'
                AND deleted_at is null
        ),
        'agents_away',
        (
            SELECT
                COUNT(*)
            FROM
                users
            WHERE
                tenant_id = $1
                AND availability_status = 'away_manual'
                AND type = 'agent'
                AND deleted_at is null
        ),
        'agents_reassigning',
        (
            SELECT
                COUNT(*)
            FROM
                users
            WHERE
                tenant_id = $1
                AND availability_status = 'away_and_reassigning'
                AND type = 'agent'
                AND deleted_at is null
        ),
        'agents_offline',
        (
            SELECT
                COUNT(*)
            FROM
                users
            WHERE
                tenant_id = $1
                AND availability_status = 'offline'
                AND type = 'agent'
                AND deleted_at is null
        )
    )
FROM
    conversations c
    INNER JOIN conversation_statuses s ON c.status_id = s.id
WHERE
    c.tenant_id = $1
    AND s.category != 'resolved';

-- name: get-overview-sla-counts
WITH first_and_resolution AS (
    SELECT
        COUNT(*) FILTER (
            WHERE
                first_response_met_at IS NOT NULL
        ) AS first_response_met_count,
        COUNT(*) FILTER (
            WHERE
                first_response_breached_at IS NOT NULL
        ) AS first_response_breached_count,
        COUNT(*) FILTER (
            WHERE
                resolution_met_at IS NOT NULL
        ) AS resolution_met_count,
        COUNT(*) FILTER (
            WHERE
                resolution_breached_at IS NOT NULL
        ) AS resolution_breached_count,
        COALESCE(
            AVG(
                EXTRACT(
                    EPOCH
                    FROM
                        (first_response_met_at - created_at)
                )
            ) FILTER (
                WHERE
                    first_response_met_at IS NOT NULL
            ),
            0
        ) AS avg_first_response_time_sec,
        COALESCE(
            AVG(
                EXTRACT(
                    EPOCH
                    FROM
                        (resolution_met_at - created_at)
                )
            ) FILTER (
                WHERE
                    resolution_met_at IS NOT NULL
            ),
            0
        ) AS avg_resolution_time_sec
    FROM
        applied_slas
    WHERE
        tenant_id = $1
        AND created_at >= CASE
            WHEN %d = 0 THEN CURRENT_DATE
            ELSE NOW() - INTERVAL '%d days'
        END
),
next_response AS (
    SELECT
        COUNT(*) FILTER (
            WHERE
                met_at IS NOT NULL
        ) AS next_response_met_count,
        COUNT(*) FILTER (
            WHERE
                breached_at IS NOT NULL
        ) AS next_response_breached_count,
        COALESCE(
            AVG(
                EXTRACT(
                    EPOCH
                    FROM
                        (met_at - created_at)
                )
            ) FILTER (
                WHERE
                    met_at IS NOT NULL
            ),
            0
        ) AS avg_next_response_time_sec
    FROM
        sla_events
    WHERE
        tenant_id = $1
        AND created_at >= CASE
            WHEN %d = 0 THEN CURRENT_DATE
            ELSE NOW() - INTERVAL '%d days'
        END
        AND type = 'next_response'
)
SELECT
    fas.first_response_met_count,
    fas.first_response_breached_count,
    fas.avg_first_response_time_sec,
    nr.next_response_met_count,
    nr.next_response_breached_count,
    nr.avg_next_response_time_sec,
    fas.resolution_met_count,
    fas.resolution_breached_count,
    fas.avg_resolution_time_sec,
    CASE
        WHEN (fas.first_response_met_count + fas.first_response_breached_count) > 0
        THEN ROUND((fas.first_response_met_count::numeric / (fas.first_response_met_count + fas.first_response_breached_count)::numeric) * 100, 1)
        ELSE 0
    END AS first_response_compliance_percent,
    CASE
        WHEN (nr.next_response_met_count + nr.next_response_breached_count) > 0
        THEN ROUND((nr.next_response_met_count::numeric / (nr.next_response_met_count + nr.next_response_breached_count)::numeric) * 100, 1)
        ELSE 0
    END AS next_response_compliance_percent,
    CASE
        WHEN (fas.resolution_met_count + fas.resolution_breached_count) > 0
        THEN ROUND((fas.resolution_met_count::numeric / (fas.resolution_met_count + fas.resolution_breached_count)::numeric) * 100, 1)
        ELSE 0
    END AS resolution_compliance_percent
FROM
    first_and_resolution fas,
    next_response nr;

-- name: get-overview-charts
WITH new_conversations AS (
    SELECT
        json_agg(row_to_json(agg)) AS data
    FROM
        (
            SELECT
                TO_CHAR(created_at :: date, 'YYYY-MM-DD') AS date,
                COUNT(*) AS count
            FROM
                conversations c
            WHERE
                c.tenant_id = $1
                AND c.created_at >= CASE
                    WHEN %d = 0 THEN CURRENT_DATE
                    ELSE NOW() - INTERVAL '%d days'
                END
            GROUP BY
                date
            ORDER BY
                date
        ) agg
),
resolved_conversations AS (
    SELECT
        json_agg(row_to_json(agg)) AS data
    FROM
        (
            SELECT
                TO_CHAR(resolved_at :: date, 'YYYY-MM-DD') AS date,
                COUNT(*) AS count
            FROM
                conversations c
            WHERE
                c.tenant_id = $1
                AND c.resolved_at IS NOT NULL
                AND c.created_at >= CASE
                    WHEN %d = 0 THEN CURRENT_DATE
                    ELSE NOW() - INTERVAL '%d days'
                END
            GROUP BY
                date
            ORDER BY
                date
        ) agg
)
SELECT
    json_build_object(
        'new_conversations',
        (
            SELECT
                data
            FROM
                new_conversations
        ),
        'resolved_conversations',
        (
            SELECT
                data
            FROM
                resolved_conversations
        )
    ) AS result;

-- name: get-overview-csat
SELECT
    json_build_object(
        'average_rating',
        COALESCE(AVG(rating) FILTER (WHERE rating > 0), 0),
        'total_responses',
        COUNT(*) FILTER (WHERE rating > 0),
        'total_sent',
        COUNT(*),
        'response_rate',
        CASE
            WHEN COUNT(*) > 0
            THEN ROUND((COUNT(*) FILTER (WHERE rating > 0)::numeric / COUNT(*)::numeric) * 100, 1)
            ELSE 0
        END
    ) AS result
FROM
    csat_responses
WHERE
    tenant_id = $1
    AND created_at >= CASE
        WHEN %d = 0 THEN CURRENT_DATE
        ELSE NOW() - INTERVAL '%d days'
    END;

-- name: get-overview-message-volume
WITH stats AS (
    SELECT
        COUNT(*) AS total,
        COUNT(*) FILTER (WHERE type = 'incoming') AS incoming,
        COUNT(*) FILTER (WHERE type = 'outgoing') AS outgoing,
        COUNT(DISTINCT conversation_id) AS convos
    FROM
        conversation_messages
    WHERE
        tenant_id = $1
        AND type IN ('incoming', 'outgoing')
        AND created_at >= CASE
            WHEN %d = 0 THEN CURRENT_DATE
            ELSE NOW() - INTERVAL '%d days'
        END
)
SELECT
    json_build_object(
        'total_messages', total,
        'incoming_messages', incoming,
        'outgoing_messages', outgoing,
        'messages_per_conversation',
        CASE
            WHEN convos > 0 THEN ROUND(total::numeric / convos::numeric, 1)
            ELSE 0
        END
    ) AS result
FROM
    stats;

-- name: get-overview-tag-distribution
WITH tag_counts AS (
    SELECT
        t.id AS tag_id,
        t.name AS tag_name,
        COUNT(ct.conversation_id) AS count
    FROM
        tags t
        LEFT JOIN conversation_tags ct ON t.id = ct.tag_id
        LEFT JOIN conversations c ON ct.conversation_id = c.id
    WHERE
        t.tenant_id = $1
        AND (c.created_at >= CASE
            WHEN %d = 0 THEN CURRENT_DATE
            ELSE NOW() - INTERVAL '%d days'
        END
        OR c.id IS NULL)
    GROUP BY
        t.id, t.name
    ORDER BY
        count DESC
    LIMIT 10
),
tagging AS (
    SELECT
        COUNT(DISTINCT c.id) FILTER (
            WHERE EXISTS (
                SELECT 1 FROM conversation_tags ct
                WHERE ct.conversation_id = c.id
            )
        ) AS tagged,
        COUNT(DISTINCT c.id) FILTER (
            WHERE NOT EXISTS (
                SELECT 1 FROM conversation_tags ct
                WHERE ct.conversation_id = c.id
            )
        ) AS untagged
    FROM
        conversations c
    WHERE
        c.tenant_id = $1
        AND c.created_at >= CASE
            WHEN %d = 0 THEN CURRENT_DATE
            ELSE NOW() - INTERVAL '%d days'
        END
)
SELECT
    json_build_object(
        'top_tags',
        COALESCE((SELECT json_agg(row_to_json(tc)) FROM tag_counts tc), '[]'::json),
        'tagged_conversations', tagged,
        'untagged_conversations', untagged,
        'tagged_percentage',
        CASE
            WHEN (tagged + untagged) > 0
            THEN ROUND((tagged::numeric / (tagged + untagged)::numeric) * 100, 1)
            ELSE 0
        END
    ) AS result
FROM
    tagging;
-- name: get-call-stats
-- Phone calls in the period: totals, per day, and per agent. $2 is days, 0
-- for today.
WITH calls AS (
    SELECT
        m.created_at,
        m.meta->'call'->>'direction' AS direction,
        m.meta->'call'->>'status' AS status,
        COALESCE((m.meta->'call'->>'duration')::int, 0) AS duration,
        NULLIF(m.meta->'call'->>'agent_id', '')::int AS agent_id,
        NULLIF(m.meta->'call'->>'agent_name', '') AS agent_name
    FROM conversation_messages m
    WHERE m.tenant_id = $1
      AND m.meta ? 'call'
      AND m.created_at >= CASE WHEN $2 = 0 THEN CURRENT_DATE ELSE NOW() - make_interval(days => $2) END
)
SELECT json_build_object(
    'totals', (SELECT json_build_object(
        'calls', COUNT(*),
        'inbound', COUNT(*) FILTER (WHERE direction = 'inbound'),
        'outbound', COUNT(*) FILTER (WHERE direction = 'outbound'),
        'answered', COUNT(*) FILTER (WHERE status = 'completed'),
        'missed', COUNT(*) FILTER (WHERE direction = 'inbound' AND status IN ('no-answer', 'busy', 'failed', 'voicemail')),
        'voicemails', COUNT(*) FILTER (WHERE status = 'voicemail'),
        'talk_seconds', COALESCE(SUM(duration) FILTER (WHERE status = 'completed'), 0),
        'avg_seconds', COALESCE(ROUND(AVG(duration) FILTER (WHERE status = 'completed')), 0)
    ) FROM calls),
    'daily', COALESCE((SELECT json_agg(d ORDER BY d.date) FROM (
        SELECT created_at::date AS date,
            COUNT(*) FILTER (WHERE direction = 'inbound') AS inbound,
            COUNT(*) FILTER (WHERE direction = 'outbound') AS outbound,
            COUNT(*) FILTER (WHERE direction = 'inbound' AND status IN ('no-answer', 'busy', 'failed', 'voicemail')) AS missed
        FROM calls GROUP BY 1
    ) d), '[]'::json),
    'agents', COALESCE((SELECT json_agg(a ORDER BY a.calls DESC) FROM (
        SELECT agent_id, MAX(agent_name) AS name,
            COUNT(*) AS calls,
            COUNT(*) FILTER (WHERE status = 'completed') AS answered,
            COALESCE(SUM(duration) FILTER (WHERE status = 'completed'), 0) AS talk_seconds
        FROM calls WHERE agent_id IS NOT NULL GROUP BY agent_id
    ) a), '[]'::json)
) AS result;

-- name: get-call-log
-- Phone calls in the period, newest first, each with its conversation and
-- the first recording on it. $2 is days, 0 for today; $3 limits to calls with
-- a recording; $4 and $5 page.
SELECT
    COUNT(*) OVER() AS total,
    m.uuid AS message_uuid,
    m.created_at,
    m.meta->'call' AS call,
    c.uuid AS conversation_uuid,
    c.reference_number,
    TRIM(CONCAT(u.first_name, ' ', u.last_name)) AS contact_name,
    rec.uuid AS recording_uuid,
    rec.filename AS recording_name,
    rec.content_type AS recording_content_type
FROM conversation_messages m
JOIN conversations c ON c.id = m.conversation_id
JOIN users u ON u.id = c.contact_id
LEFT JOIN LATERAL (
    SELECT md.uuid, md.filename, md.content_type
    FROM media md
    WHERE md.tenant_id = m.tenant_id
      AND md.model_type = 'messages'
      AND md.model_id = m.id
      AND md.content_type LIKE 'audio/%'
    ORDER BY md.id
    LIMIT 1
) rec ON TRUE
WHERE m.tenant_id = $1
  AND m.meta ? 'call'
  AND m.created_at >= CASE WHEN $2 = 0 THEN CURRENT_DATE ELSE NOW() - make_interval(days => $2) END
  AND (NOT $3 OR rec.uuid IS NOT NULL)
ORDER BY m.created_at DESC
LIMIT $4 OFFSET $5;

-- name: get-sla-breakdown
-- SLA results started in the last $2 days (0 for today), the same window and
-- counts as get-overview-sla-counts, grouped by $3: team, agent, policy, inbox
-- or priority. Team and agent are the conversation's current ones. $4 keeps
-- one metric, '' keeps all.
WITH win AS (
    SELECT CASE WHEN $2::INT = 0 THEN CURRENT_DATE::TIMESTAMPTZ ELSE NOW() - make_interval(days => $2::INT) END AS since
),
results AS (
    SELECT a.conversation_id, a.sla_policy_id, 'first_response' AS metric, a.created_at AS started_at,
        a.first_response_met_at AS met_at, a.first_response_breached_at AS breached_at
    FROM applied_slas a, win WHERE a.tenant_id = $1 AND a.created_at >= win.since
    UNION ALL
    SELECT a.conversation_id, a.sla_policy_id, 'resolution', a.created_at, a.resolution_met_at, a.resolution_breached_at
    FROM applied_slas a, win WHERE a.tenant_id = $1 AND a.created_at >= win.since
    UNION ALL
    SELECT a.conversation_id, e.sla_policy_id, 'next_response', e.created_at, e.met_at, e.breached_at
    FROM sla_events e
    JOIN applied_slas a ON a.tenant_id = e.tenant_id AND a.id = e.applied_sla_id, win
    WHERE e.tenant_id = $1 AND e.type = 'next_response' AND e.created_at >= win.since
)
SELECT
    CASE $3::TEXT
        WHEN 'team' THEN COALESCE(t.name, 'No team')
        WHEN 'agent' THEN COALESCE(NULLIF(TRIM(CONCAT(u.first_name, ' ', u.last_name)), ''), 'Unassigned')
        WHEN 'policy' THEN COALESCE(p.name, 'Deleted policy')
        WHEN 'inbox' THEN COALESCE(i.name, 'Unknown inbox')
        ELSE COALESCE(pr.name, 'No priority')
    END AS group_name,
    r.metric,
    COUNT(*) FILTER (WHERE r.met_at IS NOT NULL) AS met,
    COUNT(*) FILTER (WHERE r.breached_at IS NOT NULL) AS breached,
    COUNT(*) FILTER (WHERE r.met_at IS NULL AND r.breached_at IS NULL) AS pending,
    COALESCE(AVG(EXTRACT(EPOCH FROM (r.met_at - r.started_at))) FILTER (WHERE r.met_at IS NOT NULL), 0) AS avg_met_seconds
FROM results r
JOIN conversations c ON c.tenant_id = $1 AND c.id = r.conversation_id
LEFT JOIN teams t ON t.tenant_id = c.tenant_id AND t.id = c.assigned_team_id
LEFT JOIN users u ON u.tenant_id = c.tenant_id AND u.id = c.assigned_user_id
LEFT JOIN sla_policies p ON p.tenant_id = c.tenant_id AND p.id = r.sla_policy_id
LEFT JOIN inboxes i ON i.tenant_id = c.tenant_id AND i.id = c.inbox_id
LEFT JOIN conversation_priorities pr ON pr.tenant_id = c.tenant_id AND pr.id = c.priority_id
WHERE $4::TEXT = '' OR r.metric = $4::TEXT
GROUP BY 1, 2
ORDER BY breached DESC, group_name, r.metric;

-- name: get-sla-breaches
-- Breached SLA metrics started in the last $2 days (0 for today), newest
-- breach first, with how many there are in all. $3 keeps one metric, $4 a
-- team and $5 an agent whose name contains it; '' keeps all.
WITH win AS (
    SELECT CASE WHEN $2::INT = 0 THEN CURRENT_DATE::TIMESTAMPTZ ELSE NOW() - make_interval(days => $2::INT) END AS since
),
results AS (
    SELECT a.conversation_id, a.sla_policy_id, 'first_response' AS metric,
        a.first_response_deadline_at AS deadline_at, a.first_response_breached_at AS breached_at, a.first_response_met_at AS met_at
    FROM applied_slas a, win
    WHERE a.tenant_id = $1 AND a.created_at >= win.since AND a.first_response_breached_at IS NOT NULL
    UNION ALL
    SELECT a.conversation_id, a.sla_policy_id, 'resolution', a.resolution_deadline_at, a.resolution_breached_at, a.resolution_met_at
    FROM applied_slas a, win
    WHERE a.tenant_id = $1 AND a.created_at >= win.since AND a.resolution_breached_at IS NOT NULL
    UNION ALL
    SELECT a.conversation_id, e.sla_policy_id, 'next_response', e.deadline_at, e.breached_at, e.met_at
    FROM sla_events e
    JOIN applied_slas a ON a.tenant_id = e.tenant_id AND a.id = e.applied_sla_id, win
    WHERE e.tenant_id = $1 AND e.type = 'next_response' AND e.created_at >= win.since AND e.breached_at IS NOT NULL
)
SELECT
    COUNT(*) OVER () AS total,
    c.reference_number,
    COALESCE(c.subject, '') AS subject,
    r.metric,
    r.deadline_at,
    r.breached_at,
    r.met_at,
    COALESCE(s.name, '') AS status,
    COALESCE(t.name, '') AS team_name,
    TRIM(CONCAT(u.first_name, ' ', u.last_name)) AS agent_name,
    COALESCE(p.name, '') AS policy_name,
    c.assigned_user_id,
    c.assigned_team_id
FROM results r
JOIN conversations c ON c.tenant_id = $1 AND c.id = r.conversation_id
LEFT JOIN conversation_statuses s ON s.tenant_id = c.tenant_id AND s.id = c.status_id
LEFT JOIN teams t ON t.tenant_id = c.tenant_id AND t.id = c.assigned_team_id
LEFT JOIN users u ON u.tenant_id = c.tenant_id AND u.id = c.assigned_user_id
LEFT JOIN sla_policies p ON p.tenant_id = c.tenant_id AND p.id = r.sla_policy_id
WHERE ($3::TEXT = '' OR r.metric = $3::TEXT)
    AND ($4::TEXT = '' OR LOWER(t.name) = LOWER($4::TEXT))
    AND ($5::TEXT = '' OR POSITION(LOWER($5::TEXT) IN LOWER(CONCAT(u.first_name, ' ', u.last_name))) > 0)
ORDER BY r.breached_at DESC
LIMIT $6;

-- name: get-sla-at-risk
-- SLA deadlines not yet met or breached that fall within the next $2 minutes
-- (or have already passed), on conversations that aren't resolved, soonest
-- first, with how many there are in all.
WITH due AS (
    SELECT a.conversation_id, a.sla_policy_id, 'first_response' AS metric, a.first_response_deadline_at AS deadline_at
    FROM applied_slas a
    WHERE a.tenant_id = $1 AND a.first_response_deadline_at IS NOT NULL
        AND a.first_response_met_at IS NULL AND a.first_response_breached_at IS NULL
    UNION ALL
    SELECT a.conversation_id, a.sla_policy_id, 'resolution', a.resolution_deadline_at
    FROM applied_slas a
    WHERE a.tenant_id = $1 AND a.resolution_deadline_at IS NOT NULL
        AND a.resolution_met_at IS NULL AND a.resolution_breached_at IS NULL
    UNION ALL
    SELECT a.conversation_id, e.sla_policy_id, 'next_response', e.deadline_at
    FROM sla_events e
    JOIN applied_slas a ON a.tenant_id = e.tenant_id AND a.id = e.applied_sla_id
    WHERE e.tenant_id = $1 AND e.type = 'next_response' AND e.met_at IS NULL AND e.breached_at IS NULL
)
SELECT
    COUNT(*) OVER () AS total,
    c.reference_number,
    COALESCE(c.subject, '') AS subject,
    d.metric,
    d.deadline_at,
    s.name AS status,
    COALESCE(t.name, '') AS team_name,
    TRIM(CONCAT(u.first_name, ' ', u.last_name)) AS agent_name,
    COALESCE(p.name, '') AS policy_name,
    c.assigned_user_id,
    c.assigned_team_id
FROM due d
JOIN conversations c ON c.tenant_id = $1 AND c.id = d.conversation_id
JOIN conversation_statuses s ON s.tenant_id = c.tenant_id AND s.id = c.status_id
LEFT JOIN teams t ON t.tenant_id = c.tenant_id AND t.id = c.assigned_team_id
LEFT JOIN users u ON u.tenant_id = c.tenant_id AND u.id = c.assigned_user_id
LEFT JOIN sla_policies p ON p.tenant_id = c.tenant_id AND p.id = d.sla_policy_id
WHERE s.category <> 'resolved' AND d.deadline_at <= NOW() + make_interval(mins => $2::INT)
ORDER BY d.deadline_at
LIMIT $3;

-- name: get-sla-trend
-- Met and breached SLA metrics started in the last $2 days (0 for today), per
-- $3 ('day' or 'week') the metric started in.
WITH win AS (
    SELECT CASE WHEN $2::INT = 0 THEN CURRENT_DATE::TIMESTAMPTZ ELSE NOW() - make_interval(days => $2::INT) END AS since
),
results AS (
    SELECT 'first_response' AS metric, a.created_at AS started_at, a.first_response_met_at AS met_at, a.first_response_breached_at AS breached_at
    FROM applied_slas a, win WHERE a.tenant_id = $1 AND a.created_at >= win.since
    UNION ALL
    SELECT 'resolution', a.created_at, a.resolution_met_at, a.resolution_breached_at
    FROM applied_slas a, win WHERE a.tenant_id = $1 AND a.created_at >= win.since
    UNION ALL
    SELECT 'next_response', e.created_at, e.met_at, e.breached_at
    FROM sla_events e, win
    WHERE e.tenant_id = $1 AND e.type = 'next_response' AND e.created_at >= win.since
)
SELECT
    date_trunc($3::TEXT, r.started_at)::DATE AS period,
    r.metric,
    COUNT(*) FILTER (WHERE r.met_at IS NOT NULL) AS met,
    COUNT(*) FILTER (WHERE r.breached_at IS NOT NULL) AS breached
FROM results r
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: get-sla-policies
SELECT
    name,
    COALESCE(description, '') AS description,
    first_response_time,
    COALESCE(next_response_time, '') AS next_response_time,
    resolution_time
FROM sla_policies
WHERE tenant_id = $1
ORDER BY name;
