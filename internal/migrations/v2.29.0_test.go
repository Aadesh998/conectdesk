package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_29_0 checks the voice channel exists, an ExoPhone serves one
// workspace, and the migration can run twice.
func TestV2_29_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_voice")
	for i := range 2 {
		if err := V2_29_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	db.MustExec(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Phone', 'voice', '{"account_id":"918047112233"}')`)
	if _, err := db.Exec(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Phone again', 'voice', '{"account_id":"918047112233"}')`); err == nil {
		t.Fatal("the same number was connected twice")
	}
}
