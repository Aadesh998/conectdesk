package migrations

import (
	"encoding/json"
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_23_0 checks an enabled default tenant mailer becomes the platform's,
// and a disabled one doesn't.
func TestV2_23_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_platform_mail")
	db.MustExec(`DROP TABLE platform_settings`)
	db.MustExec(`UPDATE settings SET value = 'false' WHERE tenant_id = 1 AND key = 'notification.email.enabled'`)
	if err := V2_23_0(db, nil, nil); err != nil {
		t.Fatal(err)
	}
	var n int
	db.Get(&n, `SELECT COUNT(*) FROM platform_settings`)
	if n != 0 {
		t.Fatal("a disabled mailer was copied")
	}

	db.MustExec(`DROP TABLE platform_settings`)
	db.MustExec(`UPDATE settings SET value = 'true' WHERE tenant_id = 1 AND key = 'notification.email.enabled'`)
	db.MustExec(`UPDATE settings SET value = '"smtp.mailgun.org"' WHERE tenant_id = 1 AND key = 'notification.email.host'`)
	for i := range 2 {
		if err := V2_23_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	var raw []byte
	if err := db.Get(&raw, `SELECT value FROM platform_settings WHERE key = 'mail'`); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(raw, &m)
	if m["enabled"] != true || m["host"] != "smtp.mailgun.org" {
		t.Fatalf("copied settings: %s", raw)
	}
}
