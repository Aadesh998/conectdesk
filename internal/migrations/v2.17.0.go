package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_17_0 adds two-factor login: TOTP columns on users, and the workspace
// setting app.require_2fa, also in the seed of new workspaces.
func V2_17_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	for _, v := range []string{"agent_2fa_enabled", "agent_2fa_disabled"} {
		if _, err := db.Exec(`ALTER TYPE activity_log_type ADD VALUE IF NOT EXISTS '` + v + `'`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`
		ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT NULL;
		ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled_at TIMESTAMPTZ NULL;
		ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_recovery_codes TEXT[] NOT NULL DEFAULT '{}';
		ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_last_step BIGINT NOT NULL DEFAULT 0;

		INSERT INTO settings (tenant_id, key, value)
		SELECT id, 'app.require_2fa', 'false'::jsonb FROM tenants
		ON CONFLICT DO NOTHING;
	`); err != nil {
		return err
	}
	// New workspaces are seeded with the setting too.
	b, err := fs.Read(SeedTenantFile)
	if err != nil {
		return err
	}
	_, err = db.Exec(string(b))
	return err
}
