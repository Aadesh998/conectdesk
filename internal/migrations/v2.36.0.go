package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_36_0 drops pending_signups. Workspaces are only created by platform
// admins now, so no signup waits for its email to be verified.
func V2_36_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	_, err := db.Exec(`DROP TABLE IF EXISTS pending_signups CASCADE`)
	return err
}
