package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_27_0 checks message_deliveries is created on a database without it,
// with tenant-scoped keys, and the migration can run twice.
func TestV2_27_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_deliveries")
	db.MustExec(`DROP TABLE message_deliveries`)
	for i := range 2 {
		if err := V2_27_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	var fks int
	db.Get(&fks, `SELECT COUNT(*) FROM pg_constraint WHERE conrelid = 'message_deliveries'::regclass AND contype = 'f' AND array_length(conkey, 1) = 2`)
	if fks != 2 {
		t.Fatalf("%d tenant-scoped foreign keys, want 2", fks)
	}
}
