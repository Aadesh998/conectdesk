package callback

import (
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/callback/models"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/jmoiron/sqlx"
	"github.com/zerodha/logf"
)

const tid = 1

type fixture struct {
	db       *sqlx.DB
	m        *Manager
	inboxID  int
	contact  int
	convID   int
	agentID  int
	teamID   int
	otherCon int
	otherCv  int
}

func setup(t *testing.T) fixture {
	t.Helper()
	db := testutil.NewDB(t, "callback")
	lo := logf.New(logf.Opts{})
	m, err := New(Opts{DB: db, Lo: &lo, I18n: testutil.NewI18n(t)})
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{db: db, m: m}
	db.QueryRow(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Phone', 'voice', '{}') RETURNING id`).Scan(&f.inboxID)
	db.QueryRow(`INSERT INTO users (tenant_id, type, first_name, email, "password") VALUES (1, 'agent', 'Neha', 'neha@x.io', 'x') RETURNING id`).Scan(&f.agentID)
	db.QueryRow(`INSERT INTO teams (tenant_id, name, conversation_assignment_type) VALUES (1, 'Admissions', 'Manual') RETURNING id`).Scan(&f.teamID)
	newConv := func(name, ref string) (int, int) {
		var contact, conv int
		db.QueryRow(`INSERT INTO users (tenant_id, type, first_name, phone_number) VALUES (1, 'contact', $1, '+919876543210') RETURNING id`, name).Scan(&contact)
		db.QueryRow(`INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
			VALUES (1, $1, $2, (SELECT id FROM conversation_statuses WHERE tenant_id = 1 LIMIT 1), $3) RETURNING id`, contact, f.inboxID, ref).Scan(&conv)
		return contact, conv
	}
	f.contact, f.convID = newConv("Asha", "R1")
	f.otherCon, f.otherCv = newConv("Ravi", "R2")
	return f
}

func (f fixture) missed(t *testing.T, contact, conv int, sid string, voicemail bool, due time.Duration) (models.CallBack, bool) {
	t.Helper()
	cb, created, err := f.m.RecordMissed(tid, models.Missed{
		ContactID: contact, ConversationID: conv, InboxID: f.inboxID, Phone: "+919876543210",
		CallSID: sid, Voicemail: voicemail, DueIn: due,
	})
	if err != nil {
		t.Fatal(err)
	}
	return cb, created
}

// TestMissedCalls checks a missed call opens one call-back per contact, that
// the same call reported twice counts once and a voicemail sticks, that a
// connected call closes it and the next missed call opens a new one, and that
// unanswered call-backs count as attempts.
func TestMissedCalls(t *testing.T) {
	f := setup(t)

	cb, created := f.missed(t, f.contact, f.convID, "sid-1", false, 0)
	if !created || cb.MissedCalls != 1 || cb.Status != models.StatusPending || cb.ContactFirstName != "Asha" || cb.ReferenceNumber != "R1" {
		t.Fatalf("first missed call: created=%v %+v", created, cb)
	}
	if d := time.Until(cb.DueAt); d < 14*time.Minute || d > 16*time.Minute {
		t.Fatalf("due in %v, want the default 15 minutes", d)
	}

	// The voicemail step of the same call: no new call-back, no extra count.
	cb2, created := f.missed(t, f.contact, f.convID, "sid-1", true, 0)
	if created || cb2.ID != cb.ID || cb2.MissedCalls != 1 || !cb2.Voicemail {
		t.Fatalf("voicemail of the same call: created=%v %+v", created, cb2)
	}
	// Another call from them adds to it; the voicemail flag stays.
	cb3, created := f.missed(t, f.contact, f.convID, "sid-2", false, 0)
	if created || cb3.ID != cb.ID || cb3.MissedCalls != 2 || !cb3.Voicemail {
		t.Fatalf("second call: created=%v %+v", created, cb3)
	}

	// A call back that doesn't connect is an attempt.
	if id, _ := f.m.AddAttempt(tid, f.contact); id != cb.ID {
		t.Fatalf("attempt on %d", id)
	}
	// One that connects closes it.
	if id, _ := f.m.CloseForContact(tid, f.contact, models.ReasonCalledBack, f.agentID); id != cb.ID {
		t.Fatalf("closed %d, want %d", id, cb.ID)
	}
	done, _ := f.m.Get(tid, cb.ID)
	if done.Status != models.StatusDone || done.Attempts != 1 || done.DoneReason.String != models.ReasonCalledBack || int(done.DoneByID.Int) != f.agentID {
		t.Fatalf("closed call-back: %+v", done)
	}
	if id, _ := f.m.CloseForContact(tid, f.contact, models.ReasonAnswered, 0); id != 0 {
		t.Fatal("closed a call-back twice")
	}
	if id, _ := f.m.AddAttempt(tid, f.contact); id != 0 {
		t.Fatal("counted an attempt with nothing open")
	}

	// The next missed call is a new call-back.
	next, created := f.missed(t, f.contact, f.convID, "sid-3", false, 5*time.Minute)
	if !created || next.ID == cb.ID || next.MissedCalls != 1 {
		t.Fatalf("after closing: created=%v %+v", created, next)
	}
	if err := f.m.MarkDone(tid, next.ID, f.agentID); err != nil {
		t.Fatal(err)
	}
	if err := f.m.MarkDone(tid, next.ID, f.agentID); err == nil {
		t.Fatal("marked a done call-back done again")
	}
}

// TestScope checks agents see the call-backs of the phone conversations they
// may read, and that the list follows the conversation's assignee.
func TestScope(t *testing.T) {
	f := setup(t)
	f.missed(t, f.contact, f.convID, "a", false, 0)
	f.missed(t, f.otherCon, f.otherCv, "b", false, 0)
	f.db.MustExec(`UPDATE conversations SET assigned_user_id = $1 WHERE id = $2`, f.agentID, f.convID)
	f.db.MustExec(`UPDATE conversations SET assigned_team_id = $1 WHERE id = $2`, f.teamID, f.otherCv)

	count := func(s models.Scope, mine bool) int {
		t.Helper()
		list, total, err := f.m.List(tid, s, models.ListOpts{MineOnly: mine})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != total {
			t.Fatalf("%d rows, total %d", len(list), total)
		}
		return total
	}
	if n := count(models.Scope{All: true}, false); n != 2 {
		t.Fatalf("all: %d", n)
	}
	own := models.Scope{UserID: f.agentID, Assigned: true}
	if n := count(own, false); n != 1 {
		t.Fatalf("own: %d", n)
	}
	team := models.Scope{UserID: f.agentID, TeamIDs: []int{f.teamID}, TeamInbox: true}
	if n := count(team, false); n != 1 {
		t.Fatalf("team inbox: %d", n)
	}
	if n := count(models.Scope{UserID: 999, Unassigned: true}, false); n != 0 {
		t.Fatalf("unassigned: %d", n)
	}
	if n := count(models.Scope{All: true, UserID: f.agentID}, true); n != 1 {
		t.Fatalf("mine only: %d", n)
	}
	list, _, _ := f.m.List(tid, models.Scope{All: true}, models.ListOpts{})
	for _, cb := range list {
		if cb.ConversationID == f.convID && (cb.AssigneeName.String != "Neha" || int(cb.AssignedUserID.Int) != f.agentID) {
			t.Fatalf("assignee of the assigned one: %+v", cb)
		}
		if cb.ConversationID == f.otherCv && cb.TeamName.String != "Admissions" {
			t.Fatalf("team of the team one: %+v", cb)
		}
	}
	c, _ := f.m.Counts(tid, models.Scope{All: true})
	if c.Pending != 2 || c.Overdue != 0 {
		t.Fatalf("counts %+v", c)
	}
}

// TestOverdue checks a call-back past its due time is escalated once.
func TestOverdue(t *testing.T) {
	f := setup(t)
	cb, _ := f.missed(t, f.contact, f.convID, "a", false, time.Minute)
	f.missed(t, f.otherCon, f.otherCv, "b", false, time.Hour)
	f.db.MustExec(`UPDATE call_backs SET due_at = NOW() - INTERVAL '1 minute' WHERE id = $1`, cb.ID)

	var got []int64
	on := func(_ int, c models.CallBack) { got = append(got, c.ID) }
	f.m.escalateOverdue(on)
	f.m.escalateOverdue(on)
	if len(got) != 1 || got[0] != cb.ID {
		t.Fatalf("escalated %v, want [%d] once", got, cb.ID)
	}
	c, _ := f.m.Counts(tid, models.Scope{All: true})
	if c.Overdue != 1 {
		t.Fatalf("overdue %d", c.Overdue)
	}

	// A contact messaged since yesterday isn't messaged again.
	f.m.SetWhatsApp(tid, cb.ID, 0, "no template")
	if sent, _ := f.m.WhatsAppSentSince(tid, f.contact, time.Now().Add(-24*time.Hour)); sent {
		t.Fatal("a failed message counted as sent")
	}
	f.db.MustExec(`UPDATE call_backs SET whatsapp_sent_at = NOW() WHERE id = $1`, cb.ID)
	if sent, _ := f.m.WhatsAppSentSince(tid, f.contact, time.Now().Add(-24*time.Hour)); !sent {
		t.Fatal("a sent message wasn't found")
	}
}
