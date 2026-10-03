-- name: get-totp
SELECT COALESCE(totp_secret, '') AS secret, totp_enabled_at AS enabled_at, cardinality(totp_recovery_codes) AS recovery_codes_left
FROM users
WHERE tenant_id = $1 AND id = $2 AND type = 'agent' AND deleted_at IS NULL;

-- name: enable-totp
UPDATE users
SET totp_secret = $3, totp_enabled_at = NOW(), totp_recovery_codes = $4, totp_last_step = $5, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2 AND type = 'agent' AND deleted_at IS NULL;

-- name: disable-totp
UPDATE users
SET totp_secret = NULL, totp_enabled_at = NULL, totp_recovery_codes = '{}', totp_last_step = 0, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2 AND type = 'agent';

-- name: use-totp-step
-- Accepts a code's time step only once: a step at or before the last accepted
-- one matches no row.
UPDATE users SET totp_last_step = $3
WHERE tenant_id = $1 AND id = $2 AND totp_enabled_at IS NOT NULL AND totp_last_step < $3
RETURNING id;

-- name: use-recovery-code
UPDATE users SET totp_recovery_codes = array_remove(totp_recovery_codes, $3)
WHERE tenant_id = $1 AND id = $2 AND totp_enabled_at IS NOT NULL AND $3 = ANY(totp_recovery_codes)
RETURNING cardinality(totp_recovery_codes);

-- name: set-recovery-codes
UPDATE users SET totp_recovery_codes = $3, updated_at = NOW()
WHERE tenant_id = $1 AND id = $2 AND totp_enabled_at IS NOT NULL;
