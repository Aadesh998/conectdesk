package conversation

import (
	"os"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/knadh/goyesql/v2"
	"github.com/volatiletech/null/v9"
)

// TestLastIncomingAt checks the query WhatsApp's 24-hour window runs from:
// the contact's latest incoming message in the inbox, across their
// conversations there, ignoring replies and other inboxes.
func TestLastIncomingAt(t *testing.T) {
	db := testutil.NewDB(t, "conversation_last_incoming")
	b, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	query := q["get-last-incoming-at"].Query

	var contact, wa, mail int
	db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'contact', 'Asha') RETURNING id`)
	db.Get(&wa, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'WA', 'whatsapp', '{}') RETURNING id`)
	db.Get(&mail, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Mail', 'email', '{}') RETURNING id`)

	var at null.Time
	if err := db.Get(&at, query, 1, contact, wa); err != nil || at.Valid {
		t.Fatalf("no messages: %v %v", at, err)
	}

	conv := func(inbox int) int {
		var id int
		db.Get(&id, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
			VALUES (1, $1, $2, (SELECT id FROM conversation_statuses WHERE tenant_id = 1 LIMIT 1), gen_random_uuid()::TEXT) RETURNING id`, contact, inbox)
		return id
	}
	msg := func(conv int, typ string, hoursAgo int) {
		db.MustExec(`INSERT INTO conversation_messages (tenant_id, type, status, conversation_id, sender_id, sender_type, content, created_at)
			VALUES (1, $1, 'received', $2, $3, 'contact', 'x', NOW() - make_interval(hours => $4))`, typ, conv, contact, hoursAgo)
	}
	old, recent := conv(wa), conv(wa)
	msg(old, "incoming", 30)
	msg(recent, "incoming", 5)
	msg(recent, "outgoing", 1)
	msg(conv(mail), "incoming", 0)

	if err := db.Get(&at, query, 1, contact, wa); err != nil {
		t.Fatal(err)
	}
	if !at.Valid || time.Since(at.Time) < 4*time.Hour || time.Since(at.Time) > 6*time.Hour {
		t.Fatalf("got %v, want the incoming message from 5 hours ago", at)
	}
}
