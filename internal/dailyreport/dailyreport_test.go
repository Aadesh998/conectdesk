package dailyreport

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/jmoiron/sqlx/types"
	"github.com/zerodha/logf"
)

type memSettings struct{ s Settings }

func (m *memSettings) GetByPrefix(int, string) (types.JSONText, error) { return json.Marshal(m.s) }
func (m *memSettings) GetAppTimezone(int) string                       { return "Asia/Kolkata" }

// TestSummaryAndRun checks the day's numbers, that a report goes out once a
// day and only once its time has come, and that a failed send is recorded.
func TestSummaryAndRun(t *testing.T) {
	db := testutil.NewDB(t, "dailyreport")
	lo := logf.New(logf.Opts{})
	st := &memSettings{s: Settings{
		Enabled: true, Time: "20:00", WhatsAppInbox: 1, TemplateName: "daily", TemplateLang: "en",
		Recipients: []string{"+919800000001"},
	}}
	m, err := New(Opts{DB: db, Settings: st, WonStages: func(int) []string { return []string{"admitted"} }, Lo: &lo})
	if err != nil {
		t.Fatal(err)
	}
	db.MustExec(`UPDATE settings SET value = 'true' WHERE key = 'daily_report.enabled' AND tenant_id = 1`)

	var inbox, contact, conv int
	db.QueryRow(`INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Phone', 'voice', '{}') RETURNING id`).Scan(&inbox)
	db.QueryRow(`INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'contact', 'Asha') RETURNING id`).Scan(&contact)
	db.QueryRow(`INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
		VALUES (1, $1, $2, (SELECT id FROM conversation_statuses WHERE tenant_id = 1 LIMIT 1), 'R1') RETURNING id`, contact, inbox).Scan(&conv)
	call := func(status string) {
		db.MustExec(`INSERT INTO conversation_messages (tenant_id, conversation_id, sender_id, type, sender_type, status, content, meta)
			VALUES (1, $1, $2, 'incoming', 'contact', 'received', 'call', jsonb_build_object('call', jsonb_build_object('direction', 'inbound', 'status', $3::TEXT)))`,
			conv, contact, status)
	}
	call("completed")
	call("no-answer")
	call("voicemail")
	db.MustExec(`INSERT INTO call_backs (tenant_id, contact_id, conversation_id, inbox_id, phone, last_call_sid, due_at)
		VALUES (1, $1, $2, $3, '+919800000002', 's', NOW() - INTERVAL '1 minute')`, contact, conv, inbox)
	db.MustExec(`INSERT INTO lead_stage_events (tenant_id, contact_id, from_stage, to_stage) VALUES (1, $1, NULL, 'new'), (1, $1, 'new', 'admitted')`, contact)

	sum, err := m.Summarize(1, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Enquiries != 1 || sum.NewLeads != 1 || sum.CallsIn != 3 || sum.MissedCalls != 2 ||
		sum.PendingCallBacks != 1 || sum.OverdueCallBacks != 1 || sum.Admissions != 1 {
		t.Fatalf("summary %+v", sum)
	}
	if v := sum.Values(3); len(v) != 3 || v[1] != "1" || v[2] != "3" {
		t.Fatalf("values %v", v)
	}

	ist := time.FixedZone("IST", 5*3600+1800)
	sent := 0
	send := func(int, Settings, Summary) error { sent++; return nil }
	m.sendDue(time.Date(2026, 9, 29, 19, 59, 0, 0, ist), send)
	if sent != 0 {
		t.Fatal("sent before its time")
	}
	m.sendDue(time.Date(2026, 9, 29, 20, 0, 0, 0, ist), send)
	m.sendDue(time.Date(2026, 9, 29, 21, 0, 0, 0, ist), send)
	if sent != 1 {
		t.Fatalf("sent %d times on one day", sent)
	}
	m.sendDue(time.Date(2026, 9, 30, 20, 5, 0, 0, ist), func(int, Settings, Summary) error { return errors.New("no template") })
	runs, _ := m.LastRuns(1)
	if len(runs) != 2 || runs[0].Status != "failed" || runs[0].Error.String != "no template" || runs[1].Status != "sent" {
		t.Fatalf("runs %+v", runs)
	}
}

func TestValidate(t *testing.T) {
	ok := Settings{Enabled: true, Time: "08:30", WhatsAppInbox: 2, TemplateName: "x", TemplateLang: "en", Recipients: []string{"+91"}}
	if ok.Validate() != nil {
		t.Fatal("valid settings rejected")
	}
	for _, bad := range []Settings{
		{Time: "25:00"},
		{Enabled: true, Time: "08:30", TemplateName: "x", TemplateLang: "en", Recipients: []string{"+91"}},
		{Enabled: true, Time: "08:30", WhatsAppInbox: 2, TemplateName: "x", TemplateLang: "en"},
	} {
		if bad.Validate() == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if (Settings{Time: "08:30"}).Validate() != nil {
		t.Fatal("a report that's off needs only a time")
	}
}
