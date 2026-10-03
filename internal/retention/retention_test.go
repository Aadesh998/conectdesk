package retention

import (
	"context"
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

// Tenant 2 keeps 30 days and deletes contacts; tenant 3 keeps everything.
const acme, foo = 2, 3

func newManager(t *testing.T) (*Manager, *sqlx.DB) {
	t.Helper()
	db := testutil.NewDB(t, "retention")
	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'acme', 'Acme'), (3, 'foo', 'Foo')`)
	db.MustExec(`SELECT seed_tenant(2); SELECT seed_tenant(3)`)
	db.MustExec(`UPDATE settings SET value = '30' WHERE tenant_id = 2 AND key = 'privacy.retention_days'`)
	db.MustExec(`UPDATE settings SET value = 'true' WHERE tenant_id = 2 AND key = 'privacy.retention_delete_contacts'`)
	lo := logf.New(logf.Opts{})
	// A batch of one makes every deletion loop more than once.
	m, err := New(Opts{DB: db, Lo: &lo, BatchSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	return m, db
}

// contact inserts a contact last active daysAgo and returns its ID.
func contact(t *testing.T, db *sqlx.DB, tenantID, daysAgo int, enabled bool, avatar string) int {
	t.Helper()
	var id int
	if err := db.Get(&id, `INSERT INTO users (tenant_id, type, first_name, enabled, avatar_url, created_at, updated_at, last_active_at)
		VALUES ($1, 'contact', 'C', $2, NULLIF($3, ''), NOW() - make_interval(days => $4), NOW() - make_interval(days => $4), NOW() - make_interval(days => $4))
		RETURNING id`, tenantID, enabled, avatar, daysAgo); err != nil {
		t.Fatal(err)
	}
	return id
}

// conversation inserts a conversation in the status, last active daysAgo, and
// returns its ID.
func conversation(t *testing.T, db *sqlx.DB, tenantID, contactID int, status string, daysAgo int) int {
	t.Helper()
	var inbox int
	if err := db.Get(&inbox, `SELECT id FROM inboxes WHERE tenant_id = $1 LIMIT 1`, tenantID); err != nil {
		if err := db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES ($1, 'Mail', 'email', '{}') RETURNING id`, tenantID); err != nil {
			t.Fatal(err)
		}
	}
	var id int
	if err := db.Get(&id, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number, created_at, last_message_at)
		VALUES ($1, $2, $3, (SELECT id FROM conversation_statuses WHERE tenant_id = $1 AND name = $4), gen_random_uuid()::TEXT,
			NOW() - make_interval(days => $5), NOW() - make_interval(days => $5))
		RETURNING id`, tenantID, contactID, inbox, status, daysAgo); err != nil {
		t.Fatal(err)
	}
	db.MustExec(`INSERT INTO conversation_messages (tenant_id, type, status, conversation_id, sender_id, sender_type, content)
		VALUES ($1, 'incoming', 'received', $2, $3, 'contact', 'hello')`, tenantID, id, contactID)
	return id
}

func exists(t *testing.T, db *sqlx.DB, table string, id int) bool {
	t.Helper()
	var ok bool
	if err := db.Get(&ok, `SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1)`, id); err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestPolicies(t *testing.T) {
	m, db := newManager(t)
	// A tenant with retention off has no policy.
	db.MustExec(`UPDATE settings SET value = '0' WHERE tenant_id = 3 AND key = 'privacy.retention_days'`)
	got, err := m.Policies()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (Policy{TenantID: acme, Days: 30, DeleteContacts: true}) {
		t.Fatalf("policies = %+v", got)
	}
}

func TestApply(t *testing.T) {
	m, db := newManager(t)

	// Resolved and old: deleted, with its contact, who has nothing else.
	gone := contact(t, db, acme, 90, true, "/uploads/avatar.png")
	oldResolved := conversation(t, db, acme, gone, "Resolved", 60)
	oldClosed := conversation(t, db, acme, gone, "Closed", 45)

	// Kept: resolved recently, open however old, and their contact.
	kept := contact(t, db, acme, 90, true, "")
	recent := conversation(t, db, acme, kept, "Resolved", 10)
	oldOpen := conversation(t, db, acme, kept, "Open", 200)

	// Kept: a contact active lately, and a blocked contact, so it stays blocked.
	active := contact(t, db, acme, 5, true, "")
	blocked := contact(t, db, acme, 90, false, "")

	// Kept: another tenant's old conversation.
	other := contact(t, db, foo, 90, true, "")
	otherResolved := conversation(t, db, foo, other, "Resolved", 60)

	res, err := m.Apply(context.Background(), Policy{TenantID: acme, Days: 30, DeleteContacts: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Conversations != 2 || res.Contacts != 1 {
		t.Fatalf("deleted %d conversations and %d contacts, want 2 and 1", res.Conversations, res.Contacts)
	}
	if len(res.Avatars) != 1 || res.Avatars[0] != "/uploads/avatar.png" {
		t.Fatalf("avatars = %v", res.Avatars)
	}
	for _, id := range []int{oldResolved, oldClosed} {
		if exists(t, db, "conversations", id) {
			t.Errorf("conversation %d wasn't deleted", id)
		}
	}
	var messages int
	db.Get(&messages, `SELECT COUNT(*) FROM conversation_messages WHERE conversation_id = ANY($1)`, []int64{int64(oldResolved), int64(oldClosed)})
	if messages != 0 {
		t.Errorf("%d messages of deleted conversations are left", messages)
	}
	if exists(t, db, "users", gone) {
		t.Error("contact without conversations wasn't deleted")
	}
	for _, id := range []int{recent, oldOpen, otherResolved} {
		if !exists(t, db, "conversations", id) {
			t.Errorf("conversation %d was deleted", id)
		}
	}
	for _, id := range []int{kept, active, blocked, other} {
		if !exists(t, db, "users", id) {
			t.Errorf("contact %d was deleted", id)
		}
	}

	// Without DeleteContacts contacts stay.
	orphan := contact(t, db, acme, 90, true, "")
	if res, err := m.Apply(context.Background(), Policy{TenantID: acme, Days: 30}); err != nil || res.Contacts != 0 {
		t.Fatalf("contacts deleted without asking: %+v, %v", res, err)
	}
	if !exists(t, db, "users", orphan) {
		t.Error("contact deleted without asking")
	}
}
