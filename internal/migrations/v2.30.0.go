package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_30_0 adds the webhook kind, so a webhook can post to a Slack or Microsoft
// Teams channel instead of a custom endpoint.
func V2_30_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`ALTER TABLE webhooks ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'custom'`); err != nil {
		return err
	}
	_, err := db.Exec(`
		ALTER TABLE webhooks DROP CONSTRAINT IF EXISTS constraint_webhooks_on_kind;
		ALTER TABLE webhooks ADD CONSTRAINT constraint_webhooks_on_kind CHECK (kind IN ('custom', 'slack', 'teams'));
	`)
	return err
}
