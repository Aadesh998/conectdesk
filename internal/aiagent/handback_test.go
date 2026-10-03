package aiagent

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestReturnCandidates checks the hand-back query picks a conversation on an
// AI inbox whose customer has waited on a person past the wait, or past a
// minute on someone offline, and leaves the rest: answered, on an inbox
// without an assistant, owned by the assistant, or recently handed off.
func TestReturnCandidates(t *testing.T) {
	db := testutil.NewDB(t, "aiagent_handback")
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, db, efs); err != nil {
		t.Fatal(err)
	}

	var bot, online, offline, assistant, aiInbox, plainInbox, open int
	db.Get(&bot, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'ai_assistant', 'Juno') RETURNING id`)
	db.Get(&online, `INSERT INTO users (tenant_id, type, first_name, email, availability_status) VALUES (1, 'agent', 'On', 'on@acme.example', 'online') RETURNING id`)
	db.Get(&offline, `INSERT INTO users (tenant_id, type, first_name, email, availability_status) VALUES (1, 'agent', 'Off', 'off@acme.example', 'offline') RETURNING id`)
	db.Get(&assistant, `INSERT INTO ai_assistants (tenant_id, user_id) VALUES (1, $1) RETURNING id`, bot)
	db.Get(&aiInbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'WA AI', 'whatsapp', jsonb_build_object('ai_assistant_id', $1::int)) RETURNING id`, assistant)
	db.Get(&plainInbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'WA', 'whatsapp', '{}') RETURNING id`)
	db.Get(&open, `SELECT id FROM conversation_statuses WHERE tenant_id = 1 AND category = 'open' LIMIT 1`)

	conv := func(ref string, inbox, assignee int) (int, int) {
		var contact, id int
		db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'contact', $1) RETURNING id`, ref)
		db.Get(&id, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number, assigned_user_id)
			VALUES (1, $1, $2, $3, $4, $5) RETURNING id`, contact, inbox, open, ref, assignee)
		return id, contact
	}
	msg := func(convID, sender int, typ, senderType, ago string) {
		db.MustExec(`INSERT INTO conversation_messages (tenant_id, type, status, conversation_id, sender_id, sender_type, content, created_at)
			VALUES (1, $1, 'sent', $2, $3, $4, 'hi', now() - $5::interval)`, typ, convID, sender, senderType, ago)
	}

	waited, c1 := conv("WAITED", aiInbox, online)
	msg(waited, c1, "incoming", "contact", "6 minutes")

	fresh, c2 := conv("FRESH", aiInbox, online)
	msg(fresh, c2, "incoming", "contact", "2 minutes")

	away, c3 := conv("AWAY", aiInbox, offline)
	msg(away, c3, "incoming", "contact", "2 minutes")

	answered, c4 := conv("ANSWERED", aiInbox, online)
	msg(answered, c4, "incoming", "contact", "10 minutes")
	msg(answered, online, "outgoing", "agent", "8 minutes")

	plain, c5 := conv("PLAIN", plainInbox, online)
	msg(plain, c5, "incoming", "contact", "10 minutes")

	ai, c6 := conv("AI", aiInbox, bot)
	msg(ai, c6, "incoming", "contact", "10 minutes")

	handedOff, c7 := conv("HANDEDOFF", aiInbox, online)
	msg(handedOff, c7, "incoming", "contact", "10 minutes")
	db.MustExec(`INSERT INTO ai_agent_events (tenant_id, assistant_id, conversation_id, type) VALUES (1, $1, $2, 'handoff')`, assistant, handedOff)

	var got []returnCandidate
	if err := q.GetReturnCandidates.Select(&got, 5, 1, handoffHoldHours, 50); err != nil {
		t.Fatal(err)
	}
	ids := map[int]returnCandidate{}
	for _, c := range got {
		ids[c.ConversationID] = c
	}
	if len(got) != 2 {
		t.Fatalf("candidates = %+v, want WAITED and AWAY", got)
	}
	if c, ok := ids[waited]; !ok || c.AssistantID != assistant || c.AgentStatus != "online" {
		t.Errorf("WAITED = %+v", c)
	}
	if c, ok := ids[away]; !ok || c.AgentStatus != "offline" {
		t.Errorf("AWAY = %+v", c)
	}

	var recent bool
	if err := q.RecentHandoffExists.Get(&recent, 1, handedOff, handoffHoldHours); err != nil || !recent {
		t.Errorf("recent handoff = %v, %v", recent, err)
	}
	if err := q.RecentHandoffExists.Get(&recent, 1, waited, handoffHoldHours); err != nil || recent {
		t.Errorf("no handoff reported as recent: %v, %v", recent, err)
	}
	_ = fresh
}
