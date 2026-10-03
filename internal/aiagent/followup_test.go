package aiagent

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/testutil"
)

// TestFollowupCandidates checks the scheduler picks a conversation whose
// customer went quiet after the assistant's reply, counts follow-ups since
// the customer last wrote, and skips ones the customer answered, a human
// answered, or that aren't the assistant's.
func TestFollowupCandidates(t *testing.T) {
	db := testutil.NewDB(t, "aiagent_followups")
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, db, efs); err != nil {
		t.Fatal(err)
	}

	var bot, agent, inbox, open, assistant int
	db.Get(&bot, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'ai_assistant', 'Juno') RETURNING id`)
	db.Get(&agent, `INSERT INTO users (tenant_id, type, first_name, email) VALUES (1, 'agent', 'Asha', 'asha@acme.example') RETURNING id`)
	db.Get(&assistant, `INSERT INTO ai_assistants (tenant_id, user_id, followups_enabled) VALUES (1, $1, true) RETURNING id`, bot)
	db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'WA', 'whatsapp', '{}') RETURNING id`)
	db.Get(&open, `SELECT id FROM conversation_statuses WHERE tenant_id = 1 AND category = 'open' LIMIT 1`)

	conv := func(ref string, assignee int) (int, int) {
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

	quiet, c1 := conv("QUIET", bot)
	msg(quiet, c1, "incoming", "contact", "6 hours")
	msg(quiet, bot, "outgoing", "agent", "5 hours")

	followed, c2 := conv("FOLLOWED", bot)
	msg(followed, c2, "incoming", "contact", "30 hours")
	msg(followed, bot, "outgoing", "agent", "29 hours")
	msg(followed, bot, "outgoing", "agent", "25 hours")
	db.MustExec(`INSERT INTO ai_agent_events (tenant_id, assistant_id, conversation_id, type, created_at) VALUES (1, $1, $2, 'followup', now() - interval '25 hours')`, assistant, followed)

	answered, c3 := conv("ANSWERED", bot)
	msg(answered, bot, "outgoing", "agent", "5 hours")
	msg(answered, c3, "incoming", "contact", "4 hours")

	human, c4 := conv("HUMAN", bot)
	msg(human, c4, "incoming", "contact", "6 hours")
	msg(human, agent, "outgoing", "agent", "5 hours")

	theirs, c5 := conv("THEIRS", agent)
	msg(theirs, c5, "incoming", "contact", "6 hours")
	msg(theirs, bot, "outgoing", "agent", "5 hours")

	var got []followupCandidate
	if err := q.GetFollowupCandidates.Select(&got, 1, 50); err != nil {
		t.Fatal(err)
	}
	byConv := map[int]followupCandidate{}
	for _, c := range got {
		byConv[c.ConversationID] = c
	}
	if len(got) != 2 {
		t.Fatalf("candidates = %+v, want QUIET and FOLLOWED", got)
	}
	if c, ok := byConv[quiet]; !ok || c.FollowupsSent != 0 || c.AssistantID != assistant {
		t.Errorf("QUIET = %+v", c)
	}
	if c, ok := byConv[followed]; !ok || c.FollowupsSent != 1 {
		t.Errorf("FOLLOWED = %+v", c)
	}

	// Follow-ups don't count against the assistant's reply cap.
	db.MustExec(`UPDATE conversation_messages SET meta = '{"ai_followup": true}' WHERE conversation_id = $1 AND created_at < now() - interval '26 hours' AND type = 'outgoing'`, followed)
	var turns int
	if err := q.CountAITurns.Get(&turns, 1, followed, bot); err != nil {
		t.Fatal(err)
	}
	if turns != 1 {
		t.Errorf("turns = %d, want the one reply not marked as a follow-up", turns)
	}
}
