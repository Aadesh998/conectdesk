package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_18_0 adds two-factor login for platform admins.
func V2_18_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE platform_admins ADD COLUMN IF NOT EXISTS totp_secret TEXT NULL;
		ALTER TABLE platform_admins ADD COLUMN IF NOT EXISTS totp_enabled_at TIMESTAMPTZ NULL;
		ALTER TABLE platform_admins ADD COLUMN IF NOT EXISTS totp_recovery_codes TEXT[] NOT NULL DEFAULT '{}';
		ALTER TABLE platform_admins ADD COLUMN IF NOT EXISTS totp_last_step BIGINT NOT NULL DEFAULT 0;
	`)
	return err
}
