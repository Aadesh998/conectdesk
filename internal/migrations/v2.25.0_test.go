package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/knadh/stuffbin"
)

// TestV2_25_0 checks existing workspaces get the retention and setup
// checklist settings, keeping values they have, and the migration can run twice.
func TestV2_25_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_retention")
	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'acme', 'Acme')`)
	db.MustExec(`DELETE FROM settings WHERE key IN ('privacy.retention_days', 'privacy.retention_delete_contacts', 'onboarding.dismissed') AND tenant_id = 1`)
	db.MustExec(`INSERT INTO settings (tenant_id, key, value) VALUES (2, 'privacy.retention_days', '90')`)
	fs, err := stuffbin.NewLocalFS("/", "../../schema/890_seed_tenant.sql:"+SeedTenantFile)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if err := V2_25_0(db, fs, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	var n int
	db.Get(&n, `SELECT COUNT(*) FROM settings WHERE key IN ('privacy.retention_days', 'privacy.retention_delete_contacts', 'onboarding.dismissed')`)
	if n != 6 {
		t.Fatalf("%d settings, want 3 for each of 2 workspaces", n)
	}
	var days string
	db.Get(&days, `SELECT value::TEXT FROM settings WHERE tenant_id = 2 AND key = 'privacy.retention_days'`)
	if days != "90" {
		t.Fatalf("existing retention_days became %s", days)
	}
	db.MustExec(`INSERT INTO activity_logs (tenant_id, activity_type, activity_description, actor_id, target_model_type, target_model_id)
		SELECT 1, 'retention_applied', 'x', id, 'user', id FROM users WHERE tenant_id = 1 LIMIT 1`)
}
