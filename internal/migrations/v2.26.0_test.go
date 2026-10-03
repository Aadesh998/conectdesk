package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestV2_26_0 checks WAHA inboxes are retired with their conversations kept,
// their contacts' IDs move to the Cloud API form unless that would collide,
// an account can be connected once, and the migration can run twice.
func TestV2_26_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_meta")

	var waha, email int
	db.Get(&waha, `INSERT INTO inboxes (tenant_id, name, channel, config, secret)
		VALUES (1, 'WA', 'whatsapp', '{"base_url":"http://waha:3000","api_key":"k","session":"default"}', 'enc:x') RETURNING id`)
	db.Get(&email, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Mail', 'email', '{}') RETURNING id`)
	db.MustExec(`INSERT INTO users (tenant_id, type, first_name, external_user_id) VALUES
		(1, 'contact', 'Moved', 'whatsapp:919800000001@c.us'),
		(1, 'contact', 'Taken', 'whatsapp:919800000002@c.us'),
		(1, 'contact', 'Holder', 'whatsapp:919800000002'),
		(1, 'contact', 'Group', 'whatsapp:120363-1600000000@g.us')`)

	for i := range 2 {
		if err := V2_26_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var row struct {
		Enabled   bool   `db:"enabled"`
		Config    string `db:"config"`
		HasSecret bool   `db:"has_secret"`
	}
	db.Get(&row, `SELECT enabled, config::TEXT AS config, secret IS NOT NULL AS has_secret FROM inboxes WHERE id = $1`, waha)
	if row.Enabled || row.Config != "{}" || row.HasSecret {
		t.Fatalf("WAHA inbox not retired: %+v", row)
	}
	var emailEnabled bool
	db.Get(&emailEnabled, `SELECT enabled FROM inboxes WHERE id = $1`, email)
	if !emailEnabled {
		t.Fatal("email inbox was disabled")
	}

	ids := map[string]string{}
	rows, err := db.Query(`SELECT first_name, external_user_id FROM users WHERE type = 'contact' AND external_user_id LIKE 'whatsapp:%'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name, ext string
		rows.Scan(&name, &ext)
		ids[name] = ext
	}
	rows.Close()
	if ids["Moved"] != "whatsapp:919800000001" {
		t.Errorf("contact not moved: %q", ids["Moved"])
	}
	if ids["Taken"] != "whatsapp:919800000002@c.us" {
		t.Errorf("contact whose new ID is taken was changed: %q", ids["Taken"])
	}
	if ids["Group"] != "whatsapp:120363-1600000000@g.us" {
		t.Errorf("group chat ID changed: %q", ids["Group"])
	}

	// The new channels exist, and one account can't be connected twice.
	db.MustExec(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Page', 'messenger', '{"account_id":"P1"}')`)
	db.MustExec(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'IG', 'instagram', '{"account_id":"P1"}')`)
	if _, err := db.Exec(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Page again', 'messenger', '{"account_id":"P1"}')`); err == nil {
		t.Fatal("the same page was connected twice")
	}
}
