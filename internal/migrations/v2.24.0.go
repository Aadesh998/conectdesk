package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_24_0 renames the product to ConectDesk: workspaces still on the old
// default site name get the new one, as do new workspaces.
func V2_24_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		UPDATE settings SET value = '"ConectDesk"'::jsonb
		WHERE key = 'app.site_name' AND value IN ('"libredesk"'::jsonb, '"Libredesk"'::jsonb);
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
