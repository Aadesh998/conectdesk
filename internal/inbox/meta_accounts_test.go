package inbox

import (
	"encoding/json"
	"testing"

	imodels "github.com/conectdesk/conectdesk/internal/inbox/models"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/zerodha/logf"
)

// TestMetaAccounts covers the lookups Meta's webhook and callbacks rely on:
// finding which workspace an account belongs to, and disconnecting then
// reconnecting the inboxes a Meta user connected.
func TestMetaAccounts(t *testing.T) {
	db := testutil.NewDB(t, "inbox_meta_accounts")
	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'acme', 'Acme')`)
	lo := logf.New(logf.Opts{})
	m, err := New(&lo, db, testutil.NewI18n(t), "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}

	created, err := m.Create(2, imodels.Inbox{
		Name: "Page", Channel: ChannelMessenger, Enabled: true,
		Config: json.RawMessage(`{"account_id":"P1","page_id":"P1","access_token":"ptok","connected_by":"U9"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	// The token is encrypted at rest.
	var raw string
	db.Get(&raw, `SELECT config->>'access_token' FROM inboxes WHERE id = $1`, created.ID)
	if raw == "ptok" || raw == "" {
		t.Fatalf("access token stored as %q", raw)
	}

	tid, id, found, err := m.AccountOwner(ChannelMessenger, "P1")
	if err != nil || !found || tid != 2 || id != created.ID {
		t.Fatalf("AccountOwner: %d %d %v %v", tid, id, found, err)
	}
	if _, _, found, _ := m.AccountOwner(ChannelInstagram, "P1"); found {
		t.Fatal("account found on the wrong channel")
	}

	inboxes, err := m.ConnectedBy("U9")
	if err != nil || len(inboxes) != 1 {
		t.Fatalf("ConnectedBy: %v %v", inboxes, err)
	}

	if err := m.Disconnect(2, created.ID); err != nil {
		t.Fatal(err)
	}
	record, _ := m.GetDBRecord(2, created.ID)
	var cfg map[string]any
	json.Unmarshal(record.Config, &cfg)
	if record.Enabled || cfg["access_token"] != nil || cfg["disconnected"] != true || cfg["account_id"] != "P1" {
		t.Fatalf("after disconnect: enabled=%v config=%v", record.Enabled, cfg)
	}

	if err := m.Reconnect(2, created.ID, json.RawMessage(`{"account_id":"P1","page_id":"P1","access_token":"new"}`)); err != nil {
		t.Fatal(err)
	}
	record, _ = m.GetDBRecord(2, created.ID)
	json.Unmarshal(record.Config, &cfg)
	if !record.Enabled || cfg["access_token"] != "new" {
		t.Fatalf("after reconnect: enabled=%v config=%v", record.Enabled, cfg)
	}
}
