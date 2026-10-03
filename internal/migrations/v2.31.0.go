package migrations

import (
	"log"

	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// TenantScopedTablesV2_31_0 are the tenant tables added by V2_31_0.
var TenantScopedTablesV2_31_0 = []string{"chat_integration_threads", "chat_integrations"}

// V2_31_0 adds the Slack app and Teams bot connections and the thread each
// conversation posts to, and encrypts the URLs of Slack and Teams webhooks,
// since anyone holding one can post to the channel.
func V2_31_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS chat_integrations (
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			provider TEXT NOT NULL,
			external_team_id TEXT NOT NULL,
			external_team_name TEXT NOT NULL DEFAULT '',
			channel_id TEXT NOT NULL,
			channel_name TEXT NOT NULL DEFAULT '',
			service_url TEXT NOT NULL DEFAULT '',
			access_token TEXT NOT NULL DEFAULT '',
			bot_user_id TEXT NOT NULL DEFAULT '',
			events TEXT[] NOT NULL DEFAULT '{}',
			is_active BOOLEAN NOT NULL DEFAULT true,
			connected_by BIGINT NULL,
			CONSTRAINT constraint_chat_integrations_on_provider CHECK (provider IN ('slack', 'teams')),
			CONSTRAINT constraint_chat_integrations_on_channel_name CHECK (length(channel_name) <= 255),
			CONSTRAINT constraint_chat_integrations_on_team_name CHECK (length(external_team_name) <= 255),
			CONSTRAINT uniq_chat_integrations_tenant_id_id UNIQUE (tenant_id, id),
			-- Tenant-scoped, like every reference between tenant tables.
			FOREIGN KEY (tenant_id, connected_by) REFERENCES users (tenant_id, id) ON DELETE SET NULL (connected_by) ON UPDATE NO ACTION
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_chat_integrations_on_channel ON chat_integrations (provider, external_team_id, channel_id);
		CREATE INDEX IF NOT EXISTS index_chat_integrations_on_tenant_id ON chat_integrations (tenant_id);

		CREATE TABLE IF NOT EXISTS chat_integration_threads (
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			integration_id INT NOT NULL,
			conversation_id BIGINT NOT NULL,
			thread_ref TEXT NOT NULL,
			FOREIGN KEY (tenant_id, integration_id) REFERENCES chat_integrations (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE,
			FOREIGN KEY (tenant_id, conversation_id) REFERENCES conversations (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_chat_integration_threads_on_conversation ON chat_integration_threads (integration_id, conversation_id);
		CREATE UNIQUE INDEX IF NOT EXISTS index_chat_integration_threads_on_ref ON chat_integration_threads (integration_id, thread_ref);
		CREATE INDEX IF NOT EXISTS index_chat_integration_threads_on_tenant_id ON chat_integration_threads (tenant_id);
	`); err != nil {
		return err
	}
	return encryptChatWebhookURLs(db, ko)
}

// encryptChatWebhookURLs encrypts the URL of each Slack and Teams webhook still
// stored in the clear. Encrypted ones are left alone, so it can run twice.
func encryptChatWebhookURLs(db *sqlx.DB, ko *koanf.Koanf) error {
	if ko == nil {
		return nil
	}
	key := ko.String("app.encryption_key")
	if len(key) != 32 {
		log.Printf("skipping webhook url encryption: encryption_key is %d characters, want 32", len(key))
		return nil
	}
	var hooks []struct {
		ID  int    `db:"id"`
		URL string `db:"url"`
	}
	if err := db.Select(&hooks, `SELECT id, url FROM webhooks WHERE kind IN ('slack', 'teams')`); err != nil {
		return err
	}
	for _, h := range hooks {
		if crypto.IsEncrypted(h.URL) {
			continue
		}
		enc, err := crypto.Encrypt(h.URL, key)
		if err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE webhooks SET url = $1 WHERE id = $2`, enc, h.ID); err != nil {
			return err
		}
	}
	return nil
}
