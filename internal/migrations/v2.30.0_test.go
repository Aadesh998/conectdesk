package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_30_0 checks webhooks get a kind, existing ones stay custom, and the
// migration can run twice.
func TestV2_30_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_webhook_kind")
	db.MustExec(`ALTER TABLE webhooks DROP COLUMN IF EXISTS kind`)
	db.MustExec(`INSERT INTO webhooks (tenant_id, name, url, events) VALUES (1, 'Old', 'https://example.com/hook', '{conversation.created}')`)
	for i := range 2 {
		if err := V2_30_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	var kind string
	db.Get(&kind, `SELECT kind FROM webhooks WHERE name = 'Old'`)
	if kind != "custom" {
		t.Fatalf("existing webhook kind = %q, want custom", kind)
	}
	db.MustExec(`INSERT INTO webhooks (tenant_id, name, url, events, kind) VALUES (1, 'Slack', 'https://hooks.slack.com/services/x', '{conversation.created}', 'slack')`)
	if _, err := db.Exec(`INSERT INTO webhooks (tenant_id, name, url, events, kind) VALUES (1, 'Bad', 'https://example.com', '{conversation.created}', 'discord')`); err == nil {
		t.Fatal("an unknown kind was accepted")
	}
}
