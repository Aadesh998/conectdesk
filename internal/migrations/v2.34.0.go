package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_34_0 adds the Jev decision layer to AI assistants: the workspace's
// TypeSafe System One settings, each assistant's decision switches and
// follow-up plan, and the agent events the layer records.
func V2_34_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		ALTER TABLE ai_assistants
			ADD COLUMN IF NOT EXISTS jev_gate BOOLEAN NOT NULL DEFAULT true,
			ADD COLUMN IF NOT EXISTS jev_guard BOOLEAN NOT NULL DEFAULT true,
			ADD COLUMN IF NOT EXISTS jev_lead_stage BOOLEAN NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS jev_confidence REAL NOT NULL DEFAULT 0.7,
			ADD COLUMN IF NOT EXISTS followups_enabled BOOLEAN NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS followup_hours INTEGER[] NOT NULL DEFAULT '{4,24}',
			ADD COLUMN IF NOT EXISTS followup_templates JSONB NOT NULL DEFAULT '[]';

		ALTER TABLE ai_assistants DROP CONSTRAINT IF EXISTS constraint_ai_assistants_on_jev_confidence;
		ALTER TABLE ai_assistants ADD CONSTRAINT constraint_ai_assistants_on_jev_confidence
			CHECK (jev_confidence >= 0.5 AND jev_confidence <= 0.99);
		ALTER TABLE ai_assistants DROP CONSTRAINT IF EXISTS constraint_ai_assistants_on_followup_hours;
		ALTER TABLE ai_assistants ADD CONSTRAINT constraint_ai_assistants_on_followup_hours
			CHECK (cardinality(followup_hours) BETWEEN 1 AND 5 AND 1 <= ALL(followup_hours) AND 168 >= ALL(followup_hours));

		ALTER TABLE ai_agent_events DROP CONSTRAINT IF EXISTS constraint_ai_agent_events_on_type;
		ALTER TABLE ai_agent_events ADD CONSTRAINT constraint_ai_agent_events_on_type
			CHECK (type IN ('handoff', 'resolve', 'skip', 'block', 'followup', 'lead_stage'));
		CREATE INDEX IF NOT EXISTS index_ai_agent_events_on_conversation_type_created
			ON ai_agent_events (conversation_id, type, created_at);
	`); err != nil {
		return err
	}

	if _, err := db.Exec(`
		INSERT INTO settings (tenant_id, key, value)
		SELECT t.id, s.key, s.value::jsonb FROM tenants t, (VALUES ` + settingsV2_34_0 + `) AS s(key, value)
		ON CONFLICT DO NOTHING;
	`); err != nil {
		return err
	}

	b, err := fs.Read(SeedTenantFile)
	if err != nil {
		return err
	}
	_, err = db.Exec(string(b))
	return err
}

// settingsV2_34_0 are the settings V2_34_0 adds, as SQL VALUES rows. The seed
// of new workspaces in schema/890_seed_tenant.sql has the same ones.
const settingsV2_34_0 = `
	('jev.enabled', 'false'),
	('jev.api_key', '""'),
	('jev.model', '"jev-latest"')`
