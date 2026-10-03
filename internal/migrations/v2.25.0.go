package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_25_0 adds the workspace settings for data retention and the setup
// checklist, also in the seed of new workspaces, and the activity log types
// of retention.
func V2_25_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	for _, v := range []string{"retention_policy_changed", "retention_applied"} {
		if _, err := db.Exec(`ALTER TYPE activity_log_type ADD VALUE IF NOT EXISTS '` + v + `'`); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`
		INSERT INTO settings (tenant_id, key, value)
		SELECT t.id, s.key, s.value::jsonb FROM tenants t,
			(VALUES ('privacy.retention_days', '0'), ('privacy.retention_delete_contacts', 'false'), ('onboarding.dismissed', 'false')) AS s(key, value)
		ON CONFLICT DO NOTHING;
	`); err != nil {
		return err
	}
	b, err := fs.Read(SeedTenantFile)
	if err != nil {
		return err
	}
	_, err = db.Exec(string(b))
	return err
}
