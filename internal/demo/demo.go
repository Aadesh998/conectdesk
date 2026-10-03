// Package demo fills an empty workspace with a coaching institute's week, for
// showing the product: parents and students enquiring on WhatsApp, calls
// answered and missed, call-backs waiting and done, leads on every stage, and
// admissions. Its inboxes are switched off, so nothing is sent anywhere.
package demo

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"time"

	"github.com/jmoiron/sqlx"
)

// ErrNotEmpty is returned for a workspace that already has conversations: demo
// data is never mixed with real data.
var ErrNotEmpty = errors.New("the workspace already has conversations")

// Summary says what Seed created.
type Summary struct {
	Contacts      int
	Conversations int
	CallBacks     int
}

type dir int

const (
	in dir = iota
	out
)

// line is one message of a chat, minutes before now.
type line struct {
	dir  dir
	text string
	ago  int
}

// call is a phone call, minutes before now.
type call struct {
	inbound  bool
	status   string // completed, no-answer, voicemail
	duration int
	ago      int
	summary  string // the AI summary of an answered call
}

type contact struct {
	first, last, phone string
	attrs              map[string]any
	stage              string
	// stageAgo is when they moved onto their stage, minutes before now.
	stageAgo int
	chat     []line
	calls    []call
	// callBack is a missed call waiting, due this many minutes from now
	// (negative: overdue); callBackDone closes it.
	callBack     *int
	callBackDone string
	assign       bool
}

func ptr(n int) *int { return &n }

const (
	hour = 60
	day  = 24 * hour
)

var people = []contact{
	{
		first: "Priya", last: "Sharma", phone: "+919812345601", stage: "interested", stageAgo: 50, assign: true,
		attrs: map[string]any{"student_name": "Aarav", "class": "Class 11", "course": "JEE", "city": "Jaipur"},
		chat: []line{
			{in, "Hello, JEE ke liye aapke yahan fees kitni hai? Mera beta class 11 mein hai.", 70},
			{out, "Namaste! Our 2-year JEE programme is ₹1,20,000, and it can be paid in 4 instalments. Would you like to visit for a free counselling session?", 62},
			{in, "Ok. Weekend batch hai kya? Weekdays school hota hai.", 55},
			{out, "Yes, we have a Saturday–Sunday batch from 8 AM to 1 PM. Shall I book a visit this Saturday at 11?", 50},
		},
	},
	{
		first: "+919812345602", phone: "+919812345602", stage: "new", stageAgo: 35,
		calls:    []call{{inbound: true, status: "no-answer", ago: 35}},
		callBack: ptr(-20),
		chat: []line{
			{out, "Hi there, sorry we missed your call. A counsellor will call you back shortly. You can also reply here with your question.", 34},
			{in, "Hi, I want to know about NEET crash course for 2027", 30},
		},
	},
	{
		first: "Rakesh", last: "Verma", phone: "+919812345603", stage: "visit_booked", stageAgo: 5 * hour, assign: true,
		attrs: map[string]any{"student_name": "Ishita", "class": "Class 10", "course": "Foundation", "batch": "Evening"},
		calls: []call{{inbound: true, status: "completed", duration: 312, ago: 5*hour + 10,
			summary: "- Parent of Ishita (Class 10) asked about the Foundation course for NEET.\n- Wants an evening batch; she has school till 3 PM.\n- Agreed to visit the centre on Saturday at 11 AM with Ishita for counselling."}},
	},
	{
		first: "Sunita", last: "Patel", phone: "+919812345604", stage: "admitted", stageAgo: 2 * hour, assign: true,
		attrs: map[string]any{"student_name": "Kabir", "class": "Class 12", "course": "NEET", "city": "Ahmedabad"},
		chat: []line{
			{in, "Fee paid for Kabir's NEET batch. Please share the timetable.", 2*hour + 30},
			{out, "Thank you, Sunita ji! Welcome to the batch. The timetable and study plan are on their way.", 2 * hour},
			{in, "Thank you 🙏", 110},
		},
	},
	{
		first: "Amit", last: "Kumar", phone: "+919812345605", stage: "lost", stageAgo: 3 * day, assign: true,
		attrs: map[string]any{"class": "Class 12", "course": "JEE", "city": "Patna"},
		chat: []line{
			{in, "Fees is too high for us, we will join online coaching only.", 3*day + 20},
			{out, "Understood, Amit ji. If it helps, we also have an online batch at ₹45,000. Happy to share details anytime.", 3 * day},
		},
	},
	{
		first: "Neha", last: "Gupta", phone: "+919812345606", stage: "interested", stageAgo: 1 * day, assign: true,
		attrs: map[string]any{"course": "IELTS", "batch": "Online", "city": "Pune"},
		chat: []line{
			{in, "Do you have online IELTS classes? I work in the day.", day + 40},
			{out, "Yes! Our online IELTS batch runs Monday to Friday, 8–9:30 PM, for 6 weeks. The next batch starts on the 5th.", day + 30},
			{in, "What is the fee and is there a demo class?", day},
		},
	},
	{
		first: "+919812345607", phone: "+919812345607", stage: "new", stageAgo: 12,
		calls:    []call{{inbound: true, status: "voicemail", ago: 12, duration: 0}},
		callBack: ptr(3),
	},
	{
		first: "Vikram", last: "Singh", phone: "+919812345608", stage: "visit_booked", stageAgo: 2 * day, assign: true,
		attrs: map[string]any{"student_name": "Riya", "class": "Class 9", "course": "Foundation", "city": "Lucknow"},
		calls: []call{
			{inbound: true, status: "no-answer", ago: 2*day + 90},
			{inbound: false, status: "completed", duration: 245, ago: 2*day + 60,
				summary: "- Returned a missed call from Vikram, father of Riya (Class 9).\n- He wants a strong maths foundation before Class 10 boards.\n- Booked a campus visit for Monday at 5 PM."},
		},
		callBackDone: "called_back",
	},
	{
		first: "Farah", last: "Khan", phone: "+919812345609", stage: "admitted", stageAgo: 4 * day, assign: true,
		attrs: map[string]any{"student_name": "Zoya", "class": "Class 11", "course": "NEET", "city": "Bhopal"},
		chat: []line{
			{in, "Zoya ka admission confirm ho gaya. Hostel ka form kahan milega?", 4*day + 15},
			{out, "Congratulations! The hostel form is at the front desk, and we've also sent it here as a PDF.", 4 * day},
		},
	},
	{
		first: "Rohan", last: "Mehta", phone: "+919812345610", stage: "interested", stageAgo: 7 * hour,
		attrs: map[string]any{"course": "JEE", "class": "Class 12"},
		chat: []line{
			{in, "Is there any scholarship test for JEE dropper batch?", 7*hour + 5},
		},
	},
	{
		first: "Kavita", last: "Joshi", phone: "+919812345611", stage: "new", stageAgo: 3 * hour,
		chat: []line{
			{in, "Hi, maths tuition for class 8 hai kya? Near Sector 21.", 3 * hour},
		},
	},
	{
		first: "+919812345612", phone: "+919812345612", stage: "new", stageAgo: 90,
		calls:    []call{{inbound: true, status: "no-answer", ago: 90}, {inbound: true, status: "no-answer", ago: 75}, {inbound: false, status: "no-answer", ago: 60}},
		callBack: ptr(-60),
	},
	{
		first: "Deepak", last: "Rao", phone: "+919812345613", stage: "admitted", stageAgo: 6 * hour, assign: true,
		attrs: map[string]any{"student_name": "Ananya", "class": "Class 10", "course": "Foundation", "city": "Hyderabad"},
		calls: []call{{inbound: true, status: "completed", duration: 402, ago: 6*hour + 20,
			summary: "- Deepak confirmed Ananya's admission to the Class 10 Foundation batch.\n- Will pay the first instalment of ₹25,000 at the centre tomorrow.\n- Asked for the book list; counsellor to share it on WhatsApp."}},
	},
	{
		first: "Pooja", last: "Nair", phone: "+919812345614", stage: "lost", stageAgo: 5 * day,
		attrs: map[string]any{"course": "IELTS"},
		chat: []line{
			{in, "We have joined another institute near our home. Thanks.", 5 * day},
		},
	},
	{
		first: "Arjun", last: "Reddy", phone: "+919812345615", stage: "interested", stageAgo: 26 * hour, assign: true,
		attrs: map[string]any{"class": "Class 11", "course": "JEE", "batch": "Weekend"},
		calls: []call{{inbound: false, status: "completed", duration: 188, ago: 26 * hour,
			summary: "- Arjun is in Class 11 and wants the weekend JEE batch.\n- Asked about fees and instalments; told ₹1,20,000 in 4 parts.\n- Will discuss with parents and reply by Friday."}},
		callBackDone: "answered",
	},
	{
		first: "Meera", last: "Iyer", phone: "+919812345616", stage: "new", stageAgo: 20,
		chat: []line{
			{in, "Good evening. Is there a demo class for NEET this week?", 20},
		},
	},
}

// Seed fills the tenant's workspace with the demo data, as of now. It refuses
// a workspace with conversations.
func Seed(db *sqlx.DB, tenantID int, now time.Time) (Summary, error) {
	var sum Summary
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM conversations WHERE tenant_id = $1`, tenantID); err != nil {
		return sum, err
	}
	if n > 0 {
		return sum, ErrNotEmpty
	}

	tx, err := db.Beginx()
	if err != nil {
		return sum, err
	}
	defer tx.Rollback()
	s := seeder{tx: tx, tid: tenantID, now: now}
	if err := s.setup(); err != nil {
		return sum, err
	}
	for _, p := range people {
		if err := s.person(p, &sum); err != nil {
			return sum, fmt.Errorf("seeding %s: %w", p.first, err)
		}
	}
	return sum, tx.Commit()
}

type seeder struct {
	tx         *sqlx.Tx
	tid        int
	now        time.Time
	openID     int
	systemID   int
	agentID    int
	agentName  string
	waInbox    int
	phoneInbox int
	teamID     int
	callerID   string
}

func (s *seeder) at(minutesAgo int) time.Time {
	return s.now.Add(-time.Duration(minutesAgo) * time.Minute)
}

func (s *seeder) setup() error {
	tx, tid := s.tx, s.tid
	if err := tx.Get(&s.openID, `SELECT id FROM conversation_statuses WHERE tenant_id = $1 AND name = 'Open'`, tid); err != nil {
		return fmt.Errorf("finding the Open status: %w", err)
	}
	if err := tx.Get(&s.systemID, `SELECT id FROM users WHERE tenant_id = $1 AND type = 'agent' AND email = 'System'`, tid); err != nil {
		return fmt.Errorf("finding the system user: %w", err)
	}
	// Conversations go to the workspace's first agent, when there is one.
	var agent struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	if err := tx.Get(&agent, `SELECT id, TRIM(CONCAT(first_name, ' ', last_name)) AS name FROM users
		WHERE tenant_id = $1 AND type = 'agent' AND deleted_at IS NULL AND email <> 'System' ORDER BY id LIMIT 1`, tid); err == nil {
		s.agentID, s.agentName = agent.ID, agent.Name
	}

	s.callerID = "+918047112233"
	if err := tx.Get(&s.waInbox, `INSERT INTO inboxes (tenant_id, name, channel, enabled, config, "from")
		VALUES ($1, 'Demo: Admissions WhatsApp', 'whatsapp', FALSE, '{}', $2) RETURNING id`, tid, s.callerID); err != nil {
		return err
	}
	voiceCfg, _ := json.Marshal(map[string]any{
		"provider": "exotel", "caller_id": s.callerID, "record": true, "ring_agents": true,
		"missed_calls": map[string]any{"call_back_minutes": 15, "whatsapp_inbox_id": s.waInbox},
	})
	if err := tx.Get(&s.phoneInbox, `INSERT INTO inboxes (tenant_id, name, channel, enabled, config, "from")
		VALUES ($1, 'Demo: Admissions line', 'voice', FALSE, $2, $3) RETURNING id`, tid, voiceCfg, s.callerID); err != nil {
		return err
	}
	if err := tx.Get(&s.teamID, `INSERT INTO teams (tenant_id, name, emoji, conversation_assignment_type)
		VALUES ($1, 'Admissions counsellors', '🎓', 'Manual')
		ON CONFLICT (tenant_id, name) DO UPDATE SET updated_at = NOW() RETURNING id`, tid); err != nil {
		return err
	}
	for _, f := range [][3]string{
		{"student_name", "Student's name", "Who the enquiry is for, when a parent writes"},
		{"class", "Class", "The student's current class or grade"},
		{"course", "Course", "The course or exam they're interested in"},
		{"batch", "Preferred batch", "The batch, timing or mode they want"},
		{"city", "City", "Where the student lives or wants to attend"},
	} {
		if _, err := tx.Exec(`INSERT INTO custom_attribute_definitions (tenant_id, key, name, description, applies_to, data_type, regex, regex_hint)
			VALUES ($1, $2, $3, $4, 'contact', 'text', '', '') ON CONFLICT DO NOTHING`, tid, f[0], f[1], f[2]); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) person(p contact, sum *Summary) error {
	tx, tid := s.tx, s.tid
	attrs, _ := json.Marshal(orEmpty(p.attrs))
	var contactID int
	first := p.first
	if err := tx.Get(&contactID, `INSERT INTO users (tenant_id, type, first_name, last_name, phone_number, custom_attributes,
			lead_stage, lead_stage_changed_at, external_user_id, created_at)
		VALUES ($1, 'contact', $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		tid, first, p.last, p.phone, attrs, p.stage, s.at(p.stageAgo), "demo:"+p.phone, s.at(p.stageAgo+day)); err != nil {
		return err
	}
	sum.Contacts++

	// How they got to their stage: new first, then on.
	path := []string{"new"}
	switch p.stage {
	case "interested":
		path = append(path, "interested")
	case "visit_booked":
		path = append(path, "interested", "visit_booked")
	case "admitted":
		path = append(path, "interested", "visit_booked", "admitted")
	case "lost":
		path = append(path, "interested", "lost")
	}
	var from any
	for i, st := range path {
		at := s.at(p.stageAgo + (len(path)-1-i)*hour)
		if _, err := tx.Exec(`INSERT INTO lead_stage_events (tenant_id, contact_id, from_stage, to_stage, created_at) VALUES ($1, $2, $3, $4, $5)`,
			tid, contactID, from, st, at); err != nil {
			return err
		}
		from = st
	}

	var assignee any
	if p.assign && s.agentID > 0 {
		assignee = s.agentID
	}

	var phoneConv int
	if len(p.calls) > 0 {
		id, err := s.conversation(contactID, s.phoneInbox, "Call: "+p.phone, assignee, p.calls[0].ago)
		if err != nil {
			return err
		}
		phoneConv = id
		sum.Conversations++
		for i, c := range p.calls {
			if err := s.callMessage(phoneConv, contactID, p, c, i); err != nil {
				return err
			}
		}
	}
	var waConv, templateMsg int
	if len(p.chat) > 0 {
		id, err := s.conversation(contactID, s.waInbox, "WhatsApp: "+p.phone, assignee, p.chat[0].ago)
		if err != nil {
			return err
		}
		waConv = id
		sum.Conversations++
		for i, l := range p.chat {
			msgID, err := s.chatMessage(waConv, contactID, l)
			if err != nil {
				return err
			}
			// A chat opened by the business after a missed call starts with its template.
			if i == 0 && l.dir == out && p.callBack != nil {
				templateMsg = msgID
			}
		}
	}

	if p.callBack != nil || p.callBackDone != "" {
		missed := 0
		lastMissed := 0
		voicemail := false
		for _, c := range p.calls {
			if c.inbound && c.status != "completed" {
				missed++
				lastMissed = c.ago
				voicemail = voicemail || c.status == "voicemail"
			}
		}
		attempts := 0
		for _, c := range p.calls {
			if !c.inbound && c.status != "completed" {
				attempts++
			}
		}
		status, due, doneAt, reason := "pending", s.now, any(nil), any(nil)
		if p.callBack != nil {
			due = s.now.Add(time.Duration(*p.callBack) * time.Minute)
		} else {
			status, due, reason = "done", s.at(lastMissed-15), p.callBackDone
			doneAt = s.at(p.calls[len(p.calls)-1].ago)
		}
		var waMsg, waAt any
		if templateMsg > 0 {
			waMsg, waAt = templateMsg, s.at(lastMissed-1)
		}
		if _, err := tx.Exec(`INSERT INTO call_backs (tenant_id, contact_id, conversation_id, inbox_id, phone, status, missed_calls,
				voicemail, attempts, last_call_sid, last_missed_at, due_at, done_at, done_reason, whatsapp_message_id, whatsapp_sent_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $11)`,
			tid, contactID, phoneConv, s.phoneInbox, p.phone, status, max(missed, 1), voicemail, attempts,
			fmt.Sprintf("demo-%s-%d", p.phone, len(p.calls)-1), s.at(lastMissed), due, doneAt, reason, waMsg, waAt); err != nil {
			return err
		}
		sum.CallBacks++
	}
	return nil
}

func (s *seeder) conversation(contactID, inboxID int, subject string, assignee any, ago int) (int, error) {
	var id int
	err := s.tx.Get(&id, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number, subject,
			assigned_user_id, assigned_team_id, created_at)
		VALUES ($1, $2, $3, $4, generate_reference_number($1, ''), $5, $6, $7, $8) RETURNING id`,
		s.tid, contactID, inboxID, s.openID, subject, assignee, s.teamID, s.at(ago))
	return id, err
}

// touch keeps a conversation's latest message on it, for the list.
func (s *seeder) touch(convID int, text, sender string, senderID int, at time.Time) error {
	_, err := s.tx.Exec(`UPDATE conversations SET last_message = $2, last_message_at = $3::TIMESTAMPTZ,
			last_message_sender = $4::message_sender_type, last_message_sender_id = $5,
			last_interaction = $2, last_interaction_at = $3::TIMESTAMPTZ,
			last_interaction_sender = $4::message_sender_type, last_interaction_sender_id = $5,
			waiting_since = CASE WHEN $4::TEXT = 'contact' THEN $3::TIMESTAMPTZ END,
			last_reply_at = CASE WHEN $4::TEXT = 'agent' THEN $3::TIMESTAMPTZ ELSE last_reply_at END,
			first_reply_at = CASE WHEN $4::TEXT = 'agent' THEN COALESCE(first_reply_at, $3::TIMESTAMPTZ) ELSE first_reply_at END
		WHERE id = $1`, convID, text, at, sender, senderID)
	return err
}

func (s *seeder) chatMessage(convID, contactID int, l line) (int, error) {
	typ, sender, senderID, status := "incoming", "contact", contactID, "received"
	if l.dir == out {
		typ, sender, status = "outgoing", "agent", "sent"
		senderID = s.systemID
		if s.agentID > 0 {
			senderID = s.agentID
		}
	}
	var id int
	if err := s.tx.Get(&id, `INSERT INTO conversation_messages (tenant_id, conversation_id, type, status, content_type, content,
			text_content, sender_id, sender_type, created_at)
		VALUES ($1, $2, $3, $4, 'html', $5, $6, $7, $8, $9) RETURNING id`,
		s.tid, convID, typ, status, "<p>"+html.EscapeString(l.text)+"</p>", l.text, senderID, sender, s.at(l.ago)); err != nil {
		return 0, err
	}
	return id, s.touch(convID, l.text, sender, senderID, s.at(l.ago))
}

func (s *seeder) callMessage(convID, contactID int, p contact, c call, i int) error {
	direction, from, to := "outbound", s.callerID, p.phone
	if c.inbound {
		direction, from, to = "inbound", p.phone, s.callerID
	}
	label := callLabel(c)
	meta := map[string]any{"call": map[string]any{
		"sid": fmt.Sprintf("demo-%s-%d", p.phone, i), "provider": "exotel", "direction": direction, "status": c.status,
		"from": from, "to": to, "duration": c.duration,
		"started_at": s.at(c.ago), "ended_at": s.at(c.ago).Add(time.Duration(c.duration+20) * time.Second),
	}}
	if s.agentID > 0 && (c.status == "completed" || !c.inbound) {
		meta["call"].(map[string]any)["agent_id"] = s.agentID
		meta["call"].(map[string]any)["agent_name"] = s.agentName
	}
	if c.summary != "" {
		meta["call_ai"] = map[string]any{"status": "done", "summary": c.summary, "at": s.at(c.ago - 2)}
	}
	metaJSON, _ := json.Marshal(meta)

	typ, sender, senderID, status, private := "incoming", "contact", contactID, "received", false
	if !c.inbound {
		typ, sender, status, private = "outgoing", "agent", "sent", true
		senderID = s.systemID
		if s.agentID > 0 {
			senderID = s.agentID
		}
	}
	text := label + " " + p.phone
	if _, err := s.tx.Exec(`INSERT INTO conversation_messages (tenant_id, conversation_id, type, status, private, content_type,
			content, text_content, sender_id, sender_type, source_id, meta, created_at)
		VALUES ($1, $2, $3, $4, $5, 'html', $6, $7, $8, $9, $10, $11, $12)`,
		s.tid, convID, typ, status, private, "<p>"+html.EscapeString(text)+"</p>", text, senderID, sender,
		fmt.Sprintf("exotel:demo-%s-%d", p.phone, i), metaJSON, s.at(c.ago)); err != nil {
		return err
	}
	return s.touch(convID, text, sender, senderID, s.at(c.ago))
}

func callLabel(c call) string {
	switch c.status {
	case "completed":
		return fmt.Sprintf("Call ended · %d:%02d", c.duration/60, c.duration%60)
	case "voicemail":
		return "Voicemail"
	}
	if c.inbound {
		return "Missed call"
	}
	return "No answer"
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
