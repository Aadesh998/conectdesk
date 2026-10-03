-- Admissions: the call-back each missed call becomes, the moves of contacts
-- through the lead stages, and the owner's daily report.

DROP TABLE IF EXISTS call_backs CASCADE;
CREATE TABLE call_backs (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	contact_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE,
	-- The phone conversation the missed call is logged in.
	conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE,
	inbox_id INT NOT NULL REFERENCES inboxes(id) ON DELETE CASCADE ON UPDATE CASCADE,
	phone TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	-- How many calls were missed while it was open, and whether one left a voicemail.
	missed_calls INT NOT NULL DEFAULT 1,
	voicemail BOOLEAN NOT NULL DEFAULT FALSE,
	-- Calls back that didn't connect.
	attempts INT NOT NULL DEFAULT 0,
	last_call_sid TEXT NOT NULL,
	last_missed_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	due_at TIMESTAMPTZ NOT NULL,
	escalated_at TIMESTAMPTZ NULL,
	done_at TIMESTAMPTZ NULL,
	done_by_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
	done_reason TEXT NULL,
	-- The WhatsApp message sent to the caller about the missed call.
	whatsapp_message_id BIGINT NULL REFERENCES conversation_messages(id) ON DELETE SET NULL ON UPDATE CASCADE,
	whatsapp_sent_at TIMESTAMPTZ NULL,
	whatsapp_error TEXT NULL,
	CONSTRAINT constraint_call_backs_on_status CHECK (status IN ('pending', 'done')),
	CONSTRAINT constraint_call_backs_on_done_reason CHECK (done_reason IN ('called_back', 'answered', 'manual')),
	CONSTRAINT uniq_call_backs_tenant_id_id UNIQUE (tenant_id, id)
);
-- One open call-back per contact: another missed call adds to it.
CREATE UNIQUE INDEX uniq_call_backs_pending_contact ON call_backs (tenant_id, contact_id) WHERE status = 'pending';
CREATE INDEX index_call_backs_on_due_at ON call_backs (due_at) WHERE status = 'pending' AND escalated_at IS NULL;
CREATE INDEX index_call_backs_on_tenant_status ON call_backs (tenant_id, status, created_at DESC);
CREATE INDEX index_call_backs_on_contact_id ON call_backs (tenant_id, contact_id, created_at DESC);
CREATE INDEX index_call_backs_on_tenant_id ON call_backs (tenant_id);

DROP TABLE IF EXISTS lead_stage_events CASCADE;
CREATE TABLE lead_stage_events (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	contact_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE,
	from_stage TEXT NULL,
	to_stage TEXT NOT NULL,
	actor_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
	CONSTRAINT uniq_lead_stage_events_tenant_id_id UNIQUE (tenant_id, id)
);
CREATE INDEX index_lead_stage_events_on_tenant_created ON lead_stage_events (tenant_id, created_at);
CREATE INDEX index_lead_stage_events_on_contact_id ON lead_stage_events (tenant_id, contact_id, created_at DESC);
CREATE INDEX index_lead_stage_events_on_tenant_id ON lead_stage_events (tenant_id);

-- Each day's owner report, claimed once so only one instance sends it.
DROP TABLE IF EXISTS daily_report_runs CASCADE;
CREATE TABLE daily_report_runs (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	report_date DATE NOT NULL,
	status TEXT NOT NULL DEFAULT 'sending',
	error TEXT NULL,
	CONSTRAINT constraint_daily_report_runs_on_status CHECK (status IN ('sending', 'sent', 'failed')),
	CONSTRAINT uniq_daily_report_runs_tenant_id_id UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX uniq_daily_report_runs_on_date ON daily_report_runs (tenant_id, report_date);
CREATE INDEX index_daily_report_runs_on_tenant_id ON daily_report_runs (tenant_id);
