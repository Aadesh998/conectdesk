package conversation

import (
	"os"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/knadh/goyesql/v2"
)

// TestImportQueries checks a message can be inserted with its original time,
// and an imported conversation is dated from its first message and resolved.
func TestImportQueries(t *testing.T) {
	db := testutil.NewDB(t, "conversation_import")
	b, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}

	var contact, inbox, conv int
	db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'contact', 'Ravi') RETURNING id`)
	db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Page', 'messenger', '{}') RETURNING id`)
	db.Get(&conv, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
		VALUES (1, $1, $2, (SELECT id FROM conversation_statuses WHERE tenant_id = 1 AND category = 'open' LIMIT 1), gen_random_uuid()::TEXT) RETURNING id`, contact, inbox)

	then := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	insert := q["insert-message"].Query
	var convUUID string
	db.Get(&convUUID, `SELECT uuid FROM conversations WHERE id = $1`, conv)
	createdAt := func(content string, sentAt any) time.Time {
		t.Helper()
		if _, err := db.Exec(insert, 1, "incoming", "received", conv, convUUID, content, content, contact, "contact", false, "text", nil, `{}`, sentAt); err != nil {
			t.Fatal(err)
		}
		var at time.Time
		db.Get(&at, `SELECT created_at FROM conversation_messages WHERE content = $1`, content)
		return at
	}
	if got := createdAt("Where is my order?", then); !got.Equal(then) {
		t.Fatalf("created_at %v, want %v", got, then)
	}
	// Without a time, it's now.
	if got := createdAt("later", nil); time.Since(got) > time.Minute {
		t.Fatalf("created_at %v, want now", got)
	}

	if _, err := db.Exec(q["finish-imported-conversation"].Query, 1, conv, true); err != nil {
		t.Fatal(err)
	}
	var row struct {
		CreatedAt time.Time `db:"created_at"`
		Category  string    `db:"category"`
	}
	db.Get(&row, `SELECT c.created_at, s.category FROM conversations c JOIN conversation_statuses s ON s.id = c.status_id WHERE c.id = $1`, conv)
	if !row.CreatedAt.Equal(then) || row.Category != "resolved" {
		t.Fatalf("conversation: %+v", row)
	}
}
