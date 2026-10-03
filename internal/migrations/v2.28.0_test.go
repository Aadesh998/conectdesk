package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_28_0 checks the telegram channel exists and the migration can run twice.
func TestV2_28_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_telegram")
	for i := range 2 {
		if err := V2_28_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	db.MustExec(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Bot', 'telegram', '{"account_id":"42"}')`)
	if _, err := db.Exec(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Bot again', 'telegram', '{"account_id":"42"}')`); err == nil {
		t.Fatal("the same bot was connected twice")
	}
}
