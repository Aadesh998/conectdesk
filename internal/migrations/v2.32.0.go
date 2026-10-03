package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// TenantScopedTablesV2_32_0 are the tenant tables added by V2_32_0.
var TenantScopedTablesV2_32_0 = []string{"call_backs", "lead_stage_events", "daily_report_runs"}

// V2_32_0 sets up admissions work for institutes:
//   - users.phone_key, the trailing digits of a contact's number, so a caller
//     on the phone line and a sender on WhatsApp are matched to one contact;
//   - call_backs, the call-back task a missed call becomes, closed by the next
//     call that connects with the contact;
//   - lead stages on contacts, with lead_stage_events recording each move so
//     reports can count admissions;
//   - daily_report_runs, which claims each day's owner report once;
//   - the workspace settings for all of these, also in the seed of new
//     workspaces, and the notification types for call-backs.
func V2_32_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	for _, v := range []string{"call_back", "call_back_overdue"} {
		if _, err := db.Exec(`ALTER TYPE user_notification_type ADD VALUE IF NOT EXISTS '` + v + `'`); err != nil {
			return err
		}
	}

	if _, err := db.Exec(`
		ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_key TEXT
			GENERATED ALWAYS AS (NULLIF(RIGHT(REGEXP_REPLACE(COALESCE(phone_number, ''), '[^0-9]', '', 'g'), 10), '')) STORED;
		CREATE INDEX IF NOT EXISTS index_users_on_phone_key ON users (tenant_id, phone_key)
			WHERE type = 'contact' AND deleted_at IS NULL AND phone_key IS NOT NULL;

		ALTER TABLE users ADD COLUMN IF NOT EXISTS lead_stage TEXT NULL;
		ALTER TABLE users ADD COLUMN IF NOT EXISTS lead_stage_changed_at TIMESTAMPTZ NULL;
		DO $$ BEGIN
			ALTER TABLE users ADD CONSTRAINT constraint_users_on_lead_stage CHECK (LENGTH(lead_stage) <= 50);
		EXCEPTION WHEN duplicate_object THEN NULL;
		END $$;
		CREATE INDEX IF NOT EXISTS index_users_on_lead_stage ON users (tenant_id, lead_stage, lead_stage_changed_at DESC)
			WHERE type = 'contact' AND deleted_at IS NULL AND lead_stage IS NOT NULL;

		CREATE TABLE IF NOT EXISTS call_backs (
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			contact_id BIGINT NOT NULL,
			conversation_id BIGINT NOT NULL,
			inbox_id INT NOT NULL,
			phone TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			missed_calls INT NOT NULL DEFAULT 1,
			voicemail BOOLEAN NOT NULL DEFAULT FALSE,
			attempts INT NOT NULL DEFAULT 0,
			last_call_sid TEXT NOT NULL,
			last_missed_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			due_at TIMESTAMPTZ NOT NULL,
			escalated_at TIMESTAMPTZ NULL,
			done_at TIMESTAMPTZ NULL,
			done_by_id BIGINT NULL,
			done_reason TEXT NULL,
			whatsapp_message_id BIGINT NULL,
			whatsapp_sent_at TIMESTAMPTZ NULL,
			whatsapp_error TEXT NULL,
			CONSTRAINT constraint_call_backs_on_status CHECK (status IN ('pending', 'done')),
			CONSTRAINT constraint_call_backs_on_done_reason CHECK (done_reason IN ('called_back', 'answered', 'manual')),
			CONSTRAINT uniq_call_backs_tenant_id_id UNIQUE (tenant_id, id),
			-- Tenant-scoped, like every reference between tenant tables.
			FOREIGN KEY (tenant_id, contact_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE,
			FOREIGN KEY (tenant_id, conversation_id) REFERENCES conversations (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE,
			FOREIGN KEY (tenant_id, inbox_id) REFERENCES inboxes (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE,
			FOREIGN KEY (tenant_id, done_by_id) REFERENCES users (tenant_id, id) ON DELETE SET NULL (done_by_id) ON UPDATE CASCADE,
			FOREIGN KEY (tenant_id, whatsapp_message_id) REFERENCES conversation_messages (tenant_id, id) ON DELETE SET NULL (whatsapp_message_id) ON UPDATE CASCADE
		);
		-- One open call-back per contact: another missed call adds to it.
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_call_backs_pending_contact ON call_backs (tenant_id, contact_id) WHERE status = 'pending';
		CREATE INDEX IF NOT EXISTS index_call_backs_on_due_at ON call_backs (due_at) WHERE status = 'pending' AND escalated_at IS NULL;
		CREATE INDEX IF NOT EXISTS index_call_backs_on_tenant_status ON call_backs (tenant_id, status, created_at DESC);
		CREATE INDEX IF NOT EXISTS index_call_backs_on_contact_id ON call_backs (tenant_id, contact_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS index_call_backs_on_tenant_id ON call_backs (tenant_id);

		CREATE TABLE IF NOT EXISTS lead_stage_events (
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			contact_id BIGINT NOT NULL,
			from_stage TEXT NULL,
			to_stage TEXT NOT NULL,
			actor_id BIGINT NULL,
			CONSTRAINT uniq_lead_stage_events_tenant_id_id UNIQUE (tenant_id, id),
			FOREIGN KEY (tenant_id, contact_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE ON UPDATE CASCADE,
			FOREIGN KEY (tenant_id, actor_id) REFERENCES users (tenant_id, id) ON DELETE SET NULL (actor_id) ON UPDATE CASCADE
		);
		CREATE INDEX IF NOT EXISTS index_lead_stage_events_on_tenant_created ON lead_stage_events (tenant_id, created_at);
		CREATE INDEX IF NOT EXISTS index_lead_stage_events_on_contact_id ON lead_stage_events (tenant_id, contact_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS index_lead_stage_events_on_tenant_id ON lead_stage_events (tenant_id);

		CREATE TABLE IF NOT EXISTS daily_report_runs (
			tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			report_date DATE NOT NULL,
			status TEXT NOT NULL DEFAULT 'sending',
			error TEXT NULL,
			CONSTRAINT constraint_daily_report_runs_on_status CHECK (status IN ('sending', 'sent', 'failed')),
			CONSTRAINT uniq_daily_report_runs_tenant_id_id UNIQUE (tenant_id, id)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_daily_report_runs_on_date ON daily_report_runs (tenant_id, report_date);
		CREATE INDEX IF NOT EXISTS index_daily_report_runs_on_tenant_id ON daily_report_runs (tenant_id);
	`); err != nil {
		return err
	}

	if _, err := db.Exec(`
		INSERT INTO settings (tenant_id, key, value)
		SELECT t.id, s.key, s.value::jsonb FROM tenants t, (VALUES ` + settingsV2_32_0 + `) AS s(key, value)
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

// settingsV2_32_0 are the settings V2_32_0 adds, as SQL VALUES rows. The seed
// of new workspaces in schema/890_seed_tenant.sql has the same ones.
const settingsV2_32_0 = `
	('leads.stages', '[{"key": "new", "label": "New", "kind": "open"}, {"key": "interested", "label": "Interested", "kind": "open"}, {"key": "visit_booked", "label": "Visit booked", "kind": "open"}, {"key": "admitted", "label": "Admitted", "kind": "won"}, {"key": "lost", "label": "Lost", "kind": "lost"}]'),
	('leads.auto_new', 'true'),
	('daily_report.enabled', 'false'),
	('daily_report.time', '"20:00"'),
	('daily_report.whatsapp_inbox_id', '0'),
	('daily_report.template_name', '""'),
	('daily_report.template_language', '"en"'),
	('daily_report.recipients', '[]'),
	('ai_features.call_summaries', 'false'),
	('ai_features.lead_extraction', 'false')`
