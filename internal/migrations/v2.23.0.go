package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// PlatformTablesV2_23_0 are the platform tables added by V2_23_0.
var PlatformTablesV2_23_0 = []string{"platform_settings"}

// V2_23_0 adds platform_settings, with the platform's own mailer, set from the
// platform dashboard. Until now the default tenant's notification mailer
// served as the platform's, so an enabled one is copied over and email keeps
// going out.
func V2_23_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS platform_settings (
			key TEXT PRIMARY KEY,
			value JSONB NOT NULL DEFAULT '{}'::jsonb,
			updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
		);
		INSERT INTO platform_settings (key, value)
		SELECT 'mail', jsonb_object_agg(replace(key, 'notification.email.', ''), value)
		FROM settings
		WHERE tenant_id = 1 AND key LIKE 'notification.email.%'
		HAVING bool_or(key = 'notification.email.enabled' AND value = 'true'::jsonb)
		ON CONFLICT (key) DO NOTHING;
	`)
	return err
}
