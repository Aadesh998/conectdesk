package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_22_0 adds what re-checking verified custom domains needs. Domains
// recorded as verified without a DNS check, whose verification happened in
// the same statement as their creation, are marked trusted and not re-checked.
func V2_22_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`
		ALTER TABLE custom_domains ADD COLUMN IF NOT EXISTS failed_checks INT NOT NULL DEFAULT 0;
		ALTER TABLE custom_domains ADD COLUMN IF NOT EXISTS trusted BOOLEAN NOT NULL DEFAULT false;
		UPDATE custom_domains SET trusted = true WHERE verified_at IS NOT NULL AND verified_at = created_at;
	`)
	return err
}
