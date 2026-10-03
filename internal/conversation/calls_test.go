package conversation

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/knadh/goyesql/v2"
)

// TestCallQueries checks a call's message is found by its call ID, within
// its tenant only, and can be rewritten as the call ends.
func TestCallQueries(t *testing.T) {
	db := testutil.NewDB(t, "conversation_calls")
	b, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}

	var contact, inbox, conv, msg int
	db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'contact', '+919876543210') RETURNING id`)
	db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Phone', 'voice', '{}') RETURNING id`)
	db.Get(&conv, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
		VALUES (1, $1, $2, (SELECT id FROM conversation_statuses WHERE tenant_id = 1 LIMIT 1), gen_random_uuid()::TEXT) RETURNING id`, contact, inbox)
	db.Get(&msg, `INSERT INTO conversation_messages (tenant_id, type, status, conversation_id, sender_id, sender_type, content, source_id, meta)
		VALUES (1, 'incoming', 'received', $1, $2, 'contact', 'Incoming call', 'exotel:CALL1', '{"call":{"status":"ringing"}}') RETURNING id`, conv, contact)

	var row struct {
		ID               int             `db:"id"`
		UUID             string          `db:"uuid"`
		Meta             json.RawMessage `db:"meta"`
		ConversationUUID string          `db:"conversation_uuid"`
		ConversationID   int             `db:"conversation_id"`
	}
	if err := db.Get(&row, q["get-message-by-source-id"].Query, 1, "exotel:CALL1"); err != nil || row.ID != msg || row.ConversationID != conv {
		t.Fatalf("lookup: %+v %v", row, err)
	}
	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'other', 'Other')`)
	if err := db.Get(&row, q["get-message-by-source-id"].Query, 2, "exotel:CALL1"); err == nil {
		t.Fatal("found another tenant's call")
	}

	db.MustExec(q["update-message-content-meta"].Query, 1, msg, "<p>Missed call</p>", "Missed call", `{"call":{"status":"no-answer"}}`)
	var content, status string
	db.Get(&content, `SELECT text_content FROM conversation_messages WHERE id = $1`, msg)
	db.Get(&status, `SELECT meta->'call'->>'status' FROM conversation_messages WHERE id = $1`, msg)
	if content != "Missed call" || status != "no-answer" {
		t.Fatalf("update: %q %q", content, status)
	}
}
