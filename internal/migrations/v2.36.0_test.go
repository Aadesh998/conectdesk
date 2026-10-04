package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_36_0 upgrades a database that still has pending_signups, checks the
// table is gone, and that the migration can run twice.
func TestV2_36_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_drop_pending_signups")
	if err := V2_13_0(db, nil, nil); err != nil {
		t.Fatalf("recreating pending_signups: %v", err)
	}
	for i := range 2 {
		if err := V2_36_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	var exists bool
	db.Get(&exists, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'pending_signups')`)
	if exists {
		t.Fatal("pending_signups is still there")
	}
}
