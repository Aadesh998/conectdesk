package migrations

import (
	"log"

	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

func V2_9_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	// Add the `whatsapp` channel, backing the WAHA (WhatsApp HTTP API) inbox.
	// ADD VALUE IF NOT EXISTS keeps this re-runnable.
	if _, err := db.Exec(`ALTER TYPE "channels" ADD VALUE IF NOT EXISTS 'whatsapp'`); err != nil {
		return err
	}

	// Re-encrypt inbox secrets that an earlier update wrote back in the clear.
	// Updating an inbox preserved the secret by copying it from a record that had
	// already been decrypted for reading, and stored that copy without encrypting
	// it again, so a live chat or WhatsApp inbox saved from the admin form ended
	// up with its widget signing key or webhook token in plaintext.
	if err := encryptPlaintextInboxSecrets(db, ko); err != nil {
		return err
	}

	// Make the source ID unique so a message delivered twice cannot be stored
	// twice. The dedupe check in front of the insert loses the race when a
	// provider sends the same event twice in quick succession, which WAHA does.
	if err := dedupeMessagesBySourceID(db); err != nil {
		return err
	}

	return nil
}

// dedupeMessagesBySourceID removes messages that share a source ID, keeping the
// first of each, then enforces uniqueness with an index. Only inbound messages
// carry a source ID, so the index is partial.
func dedupeMessagesBySourceID(db *sqlx.DB) error {
	res, err := db.Exec(`
		DELETE FROM conversation_messages
		WHERE id IN (
			SELECT id FROM (
				SELECT id, row_number() OVER (PARTITION BY source_id ORDER BY id) AS rn
				FROM conversation_messages
				WHERE source_id IS NOT NULL AND source_id <> ''
			) ranked
			WHERE rn > 1
		)`)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		log.Printf("removed %d duplicate message(s) sharing a source id", rows)
	}

	if _, err := db.Exec(`DROP INDEX IF EXISTS index_conversation_messages_on_source_id`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS index_conversation_messages_on_source_id
		ON conversation_messages (source_id) WHERE source_id IS NOT NULL AND source_id <> ''`); err != nil {
		return err
	}
	return nil
}

// encryptPlaintextInboxSecrets encrypts any inbox secret still stored without
// the crypto package's prefix. Values already encrypted are left alone, so this
// is safe to run more than once.
func encryptPlaintextInboxSecrets(db *sqlx.DB, ko *koanf.Koanf) error {
	key := ko.String("app.encryption_key")
	if len(key) != 32 {
		// init enforces the key length before the app serves traffic; skipping here
		// keeps a misconfigured key from turning an upgrade into a hard failure.
		log.Printf("skipping inbox secret re-encryption: encryption_key is %d characters, want 32", len(key))
		return nil
	}

	var rows []struct {
		ID     int    `db:"id"`
		Secret string `db:"secret"`
	}
	if err := db.Select(&rows, `SELECT id, secret FROM inboxes
		WHERE secret IS NOT NULL AND secret <> '' AND secret NOT LIKE 'enc:%' AND deleted_at IS NULL`); err != nil {
		return err
	}

	for _, row := range rows {
		encrypted, err := crypto.Encrypt(row.Secret, key)
		if err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE inboxes SET secret = $2 WHERE id = $1`, row.ID, encrypted); err != nil {
			return err
		}
	}

	if len(rows) > 0 {
		log.Printf("encrypted %d plaintext inbox secret(s)", len(rows))
	}
	return nil
}
