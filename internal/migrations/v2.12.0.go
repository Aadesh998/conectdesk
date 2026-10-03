package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// SeedTenantFile holds the seed_tenant() function that creates a tenant's
// default data. It is part of the schema; this migration installs it on
// databases created before it existed.
const SeedTenantFile = "/schema/890_seed_tenant.sql"

// V2_12_0 installs seed_tenant(), which signing up uses to give each new
// tenant its default data.
func V2_12_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	b, err := fs.Read(SeedTenantFile)
	if err != nil {
		return err
	}
	_, err = db.Exec(string(b))
	return err
}
