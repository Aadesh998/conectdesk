package demo

import (
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestSeed fills an empty workspace, checks what's there, and that a second
// run is refused.
func TestSeed(t *testing.T) {
	db := testutil.NewDB(t, "demo")
	// Installing creates the system user; an agent gets the conversations.
	db.MustExec(`INSERT INTO users (tenant_id, type, email, first_name) VALUES (1, 'agent', 'System', 'System'), (1, 'agent', 'neha@x.io', 'Neha')`)
	sum, err := Seed(db, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if sum.Contacts != len(people) || sum.Conversations == 0 || sum.CallBacks == 0 {
		t.Fatalf("summary %+v", sum)
	}
	var pending, overdue, onStages, withSummary int
	db.Get(&pending, `SELECT COUNT(*) FROM call_backs WHERE tenant_id = 1 AND status = 'pending'`)
	db.Get(&overdue, `SELECT COUNT(*) FROM call_backs WHERE tenant_id = 1 AND status = 'pending' AND due_at < NOW()`)
	db.Get(&onStages, `SELECT COUNT(DISTINCT lead_stage) FROM users WHERE tenant_id = 1 AND type = 'contact'`)
	db.Get(&withSummary, `SELECT COUNT(*) FROM conversation_messages WHERE tenant_id = 1 AND meta ? 'call_ai'`)
	if pending != 3 || overdue != 2 || onStages != 5 || withSummary == 0 {
		t.Fatalf("pending %d, overdue %d, stages %d, summaries %d", pending, overdue, onStages, withSummary)
	}
	var noFrom int
	db.Get(&noFrom, `SELECT COUNT(*) FROM inboxes WHERE tenant_id = 1 AND "from" IS NULL`)
	if noFrom != 0 {
		t.Fatal("a demo inbox has no from address, which the app can't read")
	}
	if _, err := Seed(db, 1, time.Now()); err != ErrNotEmpty {
		t.Fatalf("second seed: %v", err)
	}
}
