package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_19_0 adds the workspace setting app.require_sso, also in the seed of new
// workspaces.
func V2_19_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		INSERT INTO settings (tenant_id, key, value)
		SELECT id, 'app.require_sso', 'false'::jsonb FROM tenants
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
