package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// PlatformTablesV2_15_0 are the platform tables added by V2_15_0.
var PlatformTablesV2_15_0 = []string{"platform_admins", "platform_sessions", "platform_audit_log", "platform_impersonations"}

// V2_15_0 adds the platform admin dashboard's admins, sessions, audit log and
// impersonation links, and the activity type workspaces see impersonations as.
func V2_15_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`ALTER TYPE activity_log_type ADD VALUE IF NOT EXISTS 'platform_impersonation'`); err != nil {
		return err
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS platform_admins (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			last_login_at TIMESTAMPTZ NULL,
			CONSTRAINT constraint_platform_admins_on_email CHECK (email = LOWER(email) AND length(email) <= 254)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_platform_admins_on_email ON platform_admins (email);

		CREATE TABLE IF NOT EXISTS platform_sessions (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			admin_id INT NOT NULL REFERENCES platform_admins(id) ON DELETE CASCADE,
			-- SHA-256 of the session cookie; the cookie itself is never stored.
			token_hash TEXT NOT NULL,
			ip TEXT NOT NULL DEFAULT ''
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_platform_sessions_on_token_hash ON platform_sessions (token_hash);

		CREATE TABLE IF NOT EXISTS platform_audit_log (
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			admin_id INT NULL REFERENCES platform_admins(id) ON DELETE SET NULL,
			admin_email TEXT NOT NULL,
			action TEXT NOT NULL,
			workspace_id INT NULL REFERENCES tenants(id) ON DELETE SET NULL,
			workspace_slug TEXT NOT NULL DEFAULT '',
			details JSONB NOT NULL DEFAULT '{}'::jsonb,
			ip TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS index_platform_audit_log_on_workspace_id ON platform_audit_log (workspace_id, id DESC);
		CREATE INDEX IF NOT EXISTS index_platform_audit_log_on_created_at ON platform_audit_log (created_at DESC);

		CREATE TABLE IF NOT EXISTS platform_impersonations (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			used_at TIMESTAMPTZ NULL,
			admin_id INT NOT NULL REFERENCES platform_admins(id) ON DELETE CASCADE,
			workspace_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			user_id INT NOT NULL,
			token_hash TEXT NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_platform_impersonations_on_token_hash ON platform_impersonations (token_hash);
	`)
	return err
}
