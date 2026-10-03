package report

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/knadh/goyesql/v2"
)

// TestCallStats checks the call report counts calls by direction and
// outcome, times answered calls, and credits agents, within the tenant.
func TestCallStats(t *testing.T) {
	db := testutil.NewDB(t, "report_calls")
	b, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}

	var contact, agent, inbox, conv int
	db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'contact', 'C') RETURNING id`)
	db.Get(&agent, `INSERT INTO users (tenant_id, type, first_name, email) VALUES (1, 'agent', 'Asha', 'a@acme.example') RETURNING id`)
	db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Phone', 'voice', '{}') RETURNING id`)
	db.Get(&conv, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
		VALUES (1, $1, $2, (SELECT id FROM conversation_statuses WHERE tenant_id = 1 LIMIT 1), gen_random_uuid()::TEXT) RETURNING id`, contact, inbox)
	call := func(meta string) {
		db.MustExec(`INSERT INTO conversation_messages (tenant_id, type, status, conversation_id, sender_id, sender_type, content, meta)
			VALUES (1, 'incoming', 'received', $1, $2, 'contact', 'call', $3)`, conv, contact, meta)
	}
	call(`{"call":{"direction":"inbound","status":"completed","duration":60,"agent_id":` + itoa(agent) + `,"agent_name":"Asha"}}`)
	call(`{"call":{"direction":"inbound","status":"no-answer"}}`)
	call(`{"call":{"direction":"inbound","status":"voicemail"}}`)
	call(`{"call":{"direction":"outbound","status":"completed","duration":120,"agent_id":` + itoa(agent) + `,"agent_name":"Asha"}}`)
	// Not a call.
	db.MustExec(`INSERT INTO conversation_messages (tenant_id, type, status, conversation_id, sender_id, sender_type, content)
		VALUES (1, 'incoming', 'received', $1, $2, 'contact', 'hi')`, conv, contact)

	var raw json.RawMessage
	if err := db.Get(&raw, q["get-call-stats"].Query, 1, 7); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Totals struct {
			Calls, Inbound, Outbound, Answered, Missed, Voicemails int
			TalkSeconds int `json:"talk_seconds"`
			AvgSeconds  int `json:"avg_seconds"`
		} `json:"totals"`
		Daily  []map[string]any `json:"daily"`
		Agents []struct {
			Name        string `json:"name"`
			Calls       int    `json:"calls"`
			Answered    int    `json:"answered"`
			TalkSeconds int    `json:"talk_seconds"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	tt := out.Totals
	if tt.Calls != 4 || tt.Inbound != 3 || tt.Outbound != 1 || tt.Answered != 2 || tt.Missed != 2 || tt.Voicemails != 1 || tt.TalkSeconds != 180 || tt.AvgSeconds != 90 {
		t.Fatalf("totals: %+v", tt)
	}
	if len(out.Daily) != 1 || len(out.Agents) != 1 || out.Agents[0].Name != "Asha" || out.Agents[0].Calls != 2 || out.Agents[0].TalkSeconds != 180 {
		t.Fatalf("daily %v agents %+v", out.Daily, out.Agents)
	}

	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'other', 'Other')`)
	if err := db.Get(&raw, q["get-call-stats"].Query, 2, 7); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(raw, &out)
	if out.Totals.Calls != 0 {
		t.Fatalf("other tenant sees %d calls", out.Totals.Calls)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// TestCallLog checks the call log lists a tenant's calls newest first with
// their conversation and recording, and can keep only recorded calls.
func TestCallLog(t *testing.T) {
	db := testutil.NewDB(t, "report_call_log")
	b, err := os.ReadFile("queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	q, err := goyesql.ParseBytes(b)
	if err != nil {
		t.Fatal(err)
	}

	var contact, inbox, conv int
	db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name, last_name) VALUES (1, 'contact', 'Ravi', 'K') RETURNING id`)
	db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'Phone', 'voice', '{}') RETURNING id`)
	db.Get(&conv, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
		VALUES (1, $1, $2, (SELECT id FROM conversation_statuses WHERE tenant_id = 1 LIMIT 1), 'REF-1') RETURNING id`, contact, inbox)
	call := func(meta, at string) int {
		var id int
		db.Get(&id, `INSERT INTO conversation_messages (tenant_id, type, status, conversation_id, sender_id, sender_type, content, meta, created_at)
			VALUES (1, 'incoming', 'received', $1, $2, 'contact', 'call', $3, NOW() - $4::interval) RETURNING id`, conv, contact, meta, at)
		return id
	}
	answered := call(`{"call":{"direction":"inbound","status":"completed","duration":60}}`, "2 hours")
	call(`{"call":{"direction":"inbound","status":"no-answer"}}`, "1 hour")
	db.MustExec(`INSERT INTO media (tenant_id, store, filename, content_type, model_type, model_id)
		VALUES (1, 'fs', 'call.mp3', 'audio/mpeg', 'messages', $1)`, answered)

	type row struct {
		Total           int     `db:"total"`
		ReferenceNumber string  `db:"reference_number"`
		ContactName     string  `db:"contact_name"`
		RecordingName   *string `db:"recording_name"`
	}
	var rows []row
	if err := db.Select(&rows, `SELECT total, reference_number, contact_name, recording_name FROM (`+strings.TrimSuffix(strings.TrimSpace(q["get-call-log"].Query), ";")+`) l`, 1, 7, false, 10, 0); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Total != 2 || rows[0].RecordingName != nil || rows[1].RecordingName == nil || *rows[1].RecordingName != "call.mp3" {
		t.Fatalf("log: %+v", rows)
	}
	if rows[1].ContactName != "Ravi K" || rows[1].ReferenceNumber != "REF-1" {
		t.Fatalf("conversation details: %+v", rows[1])
	}

	rows = nil
	if err := db.Select(&rows, `SELECT total, reference_number, contact_name, recording_name FROM (`+strings.TrimSuffix(strings.TrimSpace(q["get-call-log"].Query), ";")+`) l`, 1, 7, true, 10, 0); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Total != 1 {
		t.Fatalf("recorded only: %+v", rows)
	}

	db.MustExec(`INSERT INTO tenants (id, slug, name) VALUES (2, 'other', 'Other')`)
	rows = nil
	if err := db.Select(&rows, `SELECT total, reference_number, contact_name, recording_name FROM (`+strings.TrimSuffix(strings.TrimSpace(q["get-call-log"].Query), ";")+`) l`, 2, 7, false, 10, 0); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("other tenant sees %d calls", len(rows))
	}
}
