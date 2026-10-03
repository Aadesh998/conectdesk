package conversation

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/knadh/goyesql/v2"
)

func TestCombineDeliveries(t *testing.T) {
	type part = struct {
		Status string `db:"status"`
		Error  string `db:"error"`
	}
	for _, tc := range []struct {
		parts []part
		want  string
	}{
		{[]part{{Status: "read"}, {Status: "delivered"}}, "delivered"},
		{[]part{{Status: "read"}, {Status: "read"}}, "read"},
		{[]part{{Status: "read"}, {Status: "failed", Error: "x"}}, "failed"},
		{nil, ""},
	} {
		if got, _ := combineDeliveries(tc.parts); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.parts, got, tc.want)
		}
	}
}

// TestDeliveryQueries checks receipts move a message forward only, a Messenger
// watermark reads what was sent before it, and the combined state lands on
// the message's meta.
func TestDeliveryQueries(t *testing.T) {
	db := testutil.NewDB(t, "conversation_deliveries")
	b, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	sql := func(name string) string { return q[name].Query }

	var contact, inbox, conv, msg int
	db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'contact', 'Asha') RETURNING id`)
	db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Page', 'messenger', '{}') RETURNING id`)
	db.Get(&conv, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
		VALUES (1, $1, $2, (SELECT id FROM conversation_statuses WHERE tenant_id = 1 LIMIT 1), gen_random_uuid()::TEXT) RETURNING id`, contact, inbox)
	db.Get(&msg, `INSERT INTO conversation_messages (tenant_id, type, status, conversation_id, sender_id, sender_type, content, meta)
		VALUES (1, 'outgoing', 'sent', $1, $2, 'agent', 'hi', '{"echo_id":"e"}') RETURNING id`, conv, contact)

	for _, id := range []string{"m_1", "m_2", "m_1"} {
		db.MustExec(sql("insert-message-delivery"), 1, msg, inbox, "5551", id)
	}

	apply := func(ext, status string) []int {
		var ids []int
		if err := db.Select(&ids, sql("apply-message-delivery"), 1, inbox, ext, status, ""); err != nil {
			t.Fatal(err)
		}
		return ids
	}
	if ids := apply("m_1", "read"); len(ids) != 1 || ids[0] != msg {
		t.Fatalf("read: %v", ids)
	}
	// A late "delivered" doesn't move m_1 back from read.
	if ids := apply("m_1", "delivered"); len(ids) != 0 {
		t.Fatalf("moved back: %v", ids)
	}
	// Another inbox's receipt for the same ID changes nothing.
	var other []int
	db.Select(&other, sql("apply-message-delivery"), 1, inbox+1000, "m_2", "read", "")
	if len(other) != 0 {
		t.Fatalf("other inbox applied: %v", other)
	}

	var ids []int
	if err := db.Select(&ids, sql("apply-message-read-watermark"), 1, inbox, "5551", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("watermark read %v, want only m_2", ids)
	}

	var parts []struct {
		Status string `db:"status"`
		Error  string `db:"error"`
	}
	db.Select(&parts, sql("get-message-deliveries"), 1, msg)
	if status, _ := combineDeliveries(parts); status != "read" || len(parts) != 2 {
		t.Fatalf("combined %q from %v", status, parts)
	}

	var row struct {
		UUID             string          `db:"uuid"`
		Status           string          `db:"status"`
		Meta             json.RawMessage `db:"meta"`
		ConversationUUID string          `db:"conversation_uuid"`
	}
	if err := db.Get(&row, sql("set-message-delivery"), 1, msg, `{"status":"read"}`); err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	json.Unmarshal(row.Meta, &meta)
	if meta["echo_id"] != "e" || meta["delivery"].(map[string]any)["status"] != "read" || row.ConversationUUID == "" {
		t.Fatalf("meta: %s", row.Meta)
	}
}
