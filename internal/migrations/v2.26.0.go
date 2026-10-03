package migrations

import (
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_26_0 moves WhatsApp onto Meta's Cloud API and adds the Messenger and
// Instagram channels, all connected through the platform's Meta app.
//
// WhatsApp inboxes on the WAHA bridge can't carry over: the Cloud API number
// has to be connected again through Embedded Signup. They are disabled with
// their bridge settings cleared, and keep their conversations. Their contacts'
// IDs are rewritten from WAHA's chat ID ("whatsapp:919999999999@c.us") to the
// Cloud API's WhatsApp ID ("whatsapp:919999999999"), so a contact who writes to
// the reconnected number lands on the contact and history they already have.
func V2_26_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	for _, v := range []string{"messenger", "instagram"} {
		if _, err := db.Exec(`ALTER TYPE "channels" ADD VALUE IF NOT EXISTS '` + v + `'`); err != nil {
			return err
		}
	}

	res, err := db.Exec(`
		UPDATE inboxes SET enabled = false, secret = NULL, config = '{}'::jsonb, updated_at = now()
		WHERE channel = 'whatsapp' AND config->>'base_url' IS NOT NULL`)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("disabled %d WAHA WhatsApp inbox(es); reconnect them through Meta's Embedded Signup", n)
	}

	// A contact is left alone if the rewritten ID is already taken in its
	// tenant, which the unique index on external IDs would refuse anyway.
	if _, err := db.Exec(`
		UPDATE users u SET external_user_id = regexp_replace(u.external_user_id, '@c\.us$', '')
		WHERE u.type = 'contact' AND u.external_user_id LIKE 'whatsapp:%@c.us'
		AND NOT EXISTS (
			SELECT 1 FROM users o
			WHERE o.tenant_id = u.tenant_id AND o.type = 'contact' AND o.deleted_at IS NULL
			AND o.external_user_id = regexp_replace(u.external_user_id, '@c\.us$', '')
		)`); err != nil {
		return err
	}

	// Webhooks name only the account, so an account can be connected to one
	// workspace. The index enforces that and serves the lookup that routes them.
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS index_inboxes_on_channel_account
		ON inboxes (channel, (config->>'account_id'))
		WHERE deleted_at IS NULL AND config->>'account_id' IS NOT NULL`); err != nil {
		return err
	}
	return nil
}
