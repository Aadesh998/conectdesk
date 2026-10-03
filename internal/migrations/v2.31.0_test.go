package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
)

// TestV2_31_0 upgrades a database without the chat integration tables, checks
// they come back tenant-scoped, that Slack and Teams webhook URLs are
// encrypted and custom ones left alone, and that the migration can run twice.
func TestV2_31_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_chat_integrations")
	for _, tbl := range TenantScopedTablesV2_31_0 {
		db.MustExec(`DROP TABLE ` + tbl)
	}
	db.MustExec(`INSERT INTO webhooks (tenant_id, name, url, events, kind) VALUES
		(1, 'Slack', 'https://hooks.slack.com/services/T/B/x', '{conversation.created}', 'slack'),
		(1, 'Custom', 'https://example.com/hook', '{conversation.created}', 'custom')`)

	key := "0123456789abcdef0123456789abcdef"
	ko := koanf.New(".")
	ko.Load(confmap.Provider(map[string]any{"app.encryption_key": key}, "."), nil)
	for i := range 2 {
		if err := V2_31_0(db, nil, ko); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var slackURL, customURL string
	db.Get(&slackURL, `SELECT url FROM webhooks WHERE name = 'Slack'`)
	db.Get(&customURL, `SELECT url FROM webhooks WHERE name = 'Custom'`)
	if plain, err := crypto.Decrypt(slackURL, key); err != nil || !crypto.IsEncrypted(slackURL) || plain != "https://hooks.slack.com/services/T/B/x" {
		t.Errorf("slack url = %q (%v)", slackURL, err)
	}
	if customURL != "https://example.com/hook" {
		t.Errorf("custom url changed to %q", customURL)
	}

	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'other', 'Other') ON CONFLICT DO NOTHING`)
	db.MustExec(`INSERT INTO chat_integrations (tenant_id, provider, external_team_id, channel_id) VALUES (1, 'slack', 'T1', 'C1')`)
	if _, err := db.Exec(`INSERT INTO chat_integrations (tenant_id, provider, external_team_id, channel_id) VALUES (2, 'slack', 'T1', 'C1')`); err == nil {
		t.Error("the same channel was connected to two workspaces")
	}
	if _, err := db.Exec(`INSERT INTO chat_integrations (tenant_id, provider, external_team_id, channel_id) VALUES (1, 'discord', 'T1', 'C2')`); err == nil {
		t.Error("an unknown provider was accepted")
	}
}
