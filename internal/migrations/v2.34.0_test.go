package migrations

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/knadh/stuffbin"
)

// TestV2_34_0 upgrades a database from before the Jev layer, checks the
// assistant columns, event types and settings come back with their defaults
// and limits, and that the migration can run twice.
func TestV2_34_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_jev")
	db.MustExec(`
		ALTER TABLE ai_assistants DROP COLUMN jev_gate, DROP COLUMN jev_guard, DROP COLUMN jev_lead_stage,
			DROP COLUMN jev_confidence, DROP COLUMN followups_enabled, DROP COLUMN followup_hours, DROP COLUMN followup_templates;
		ALTER TABLE ai_agent_events DROP CONSTRAINT constraint_ai_agent_events_on_type;
		ALTER TABLE ai_agent_events ADD CONSTRAINT constraint_ai_agent_events_on_type CHECK (type IN ('handoff', 'resolve'));
		DROP INDEX index_ai_agent_events_on_conversation_type_created;
		DELETE FROM settings WHERE key LIKE 'jev.%';
	`)
	fs, err := stuffbin.NewLocalFS("/", "../../schema/890_seed_tenant.sql:"+SeedTenantFile)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if err := V2_34_0(db, fs, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var user, assistant, conv int
	db.Get(&user, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'ai_assistant', 'Juno') RETURNING id`)
	db.Get(&assistant, `INSERT INTO ai_assistants (tenant_id, user_id) VALUES (1, $1) RETURNING id`, user)
	var defaults struct {
		Gate       bool    `db:"jev_gate"`
		Guard      bool    `db:"jev_guard"`
		LeadStage  bool    `db:"jev_lead_stage"`
		Confidence float64 `db:"jev_confidence"`
		Followups  bool    `db:"followups_enabled"`
		Hours      string  `db:"followup_hours"`
		Templates  string  `db:"followup_templates"`
	}
	if err := db.Get(&defaults, `SELECT jev_gate, jev_guard, jev_lead_stage, jev_confidence, followups_enabled, followup_hours::text, followup_templates::text FROM ai_assistants WHERE id = $1`, assistant); err != nil {
		t.Fatal(err)
	}
	if !defaults.Gate || !defaults.Guard || defaults.LeadStage || defaults.Followups || defaults.Hours != "{4,24}" || defaults.Templates != "[]" {
		t.Fatalf("defaults = %+v", defaults)
	}
	if _, err := db.Exec(`UPDATE ai_assistants SET jev_confidence = 0.3 WHERE id = $1`, assistant); err == nil {
		t.Error("a confidence below 0.5 was accepted")
	}
	if _, err := db.Exec(`UPDATE ai_assistants SET followup_hours = '{4,500}' WHERE id = $1`, assistant); err == nil {
		t.Error("a follow-up more than a week out was accepted")
	}

	var contact, inbox int
	db.Get(&contact, `INSERT INTO users (tenant_id, type, first_name) VALUES (1, 'contact', 'Ravi') RETURNING id`)
	db.Get(&inbox, `INSERT INTO inboxes (tenant_id, name, channel, config) VALUES (1, 'WA', 'whatsapp', '{}') RETURNING id`)
	if err := db.Get(&conv, `INSERT INTO conversations (tenant_id, contact_id, inbox_id, status_id, reference_number)
		VALUES (1, $1, $2, (SELECT id FROM conversation_statuses WHERE tenant_id = 1 LIMIT 1), 'JEV-1') RETURNING id`, contact, inbox); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"skip", "block", "followup", "lead_stage"} {
		if _, err := db.Exec(`INSERT INTO ai_agent_events (tenant_id, assistant_id, conversation_id, type) VALUES (1, $1, $2, $3)`, assistant, conv, typ); err != nil {
			t.Errorf("event %s refused: %v", typ, err)
		}
	}

	var n int
	db.Get(&n, `SELECT COUNT(*) FROM settings WHERE tenant_id = 1 AND key = ANY('{jev.enabled,jev.api_key,jev.model}')`)
	if n != 3 {
		t.Fatalf("jev settings = %d, want 3", n)
	}
}
