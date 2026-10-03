-- SLA tracking: policies applied to conversations and their deadlines.

DROP TABLE IF EXISTS applied_slas CASCADE;
CREATE TABLE applied_slas (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),

	status applied_sla_status DEFAULT 'pending' NOT NULL,

	-- Cascade deletes when conversation or SLA policy is deleted.
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	sla_policy_id INT REFERENCES sla_policies(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,

	first_response_deadline_at TIMESTAMPTZ NULL,
	resolution_deadline_at TIMESTAMPTZ NULL,
	first_response_breached_at TIMESTAMPTZ NULL,
	resolution_breached_at TIMESTAMPTZ NULL,
	first_response_met_at TIMESTAMPTZ NULL,
	resolution_met_at TIMESTAMPTZ NULL
);
CREATE INDEX index_applied_slas_on_conversation_id ON applied_slas(conversation_id);
CREATE INDEX index_applied_slas_on_status ON applied_slas(status);
CREATE UNIQUE INDEX index_applied_slas_unique_pending_per_conv ON applied_slas(conversation_id) WHERE status = 'pending';
CREATE INDEX index_applied_slas_on_tenant_id ON applied_slas (tenant_id);

DROP TABLE IF EXISTS sla_events CASCADE;
CREATE TABLE sla_events (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	status sla_event_status DEFAULT 'pending' NOT NULL,
	applied_sla_id BIGINT REFERENCES applied_slas(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	sla_policy_id INT REFERENCES sla_policies(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	type sla_metric NOT NULL,
	deadline_at TIMESTAMPTZ NOT NULL,
	met_at TIMESTAMPTZ,
	breached_at TIMESTAMPTZ
);
CREATE INDEX index_sla_events_on_applied_sla_id ON sla_events(applied_sla_id);
CREATE INDEX index_sla_events_on_status ON sla_events(status);
CREATE INDEX index_sla_events_on_tenant_id ON sla_events (tenant_id);

DROP TABLE IF EXISTS scheduled_sla_notifications CASCADE;
CREATE TABLE scheduled_sla_notifications (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  applied_sla_id BIGINT NOT NULL REFERENCES applied_slas(id) ON DELETE CASCADE,
  sla_event_id BIGINT REFERENCES sla_events(id) ON DELETE CASCADE,
  metric sla_metric NOT NULL,
  notification_type sla_notification_type NOT NULL,
  recipients TEXT[] NOT NULL,
  send_at TIMESTAMPTZ NOT NULL,
  processed_at TIMESTAMPTZ
);
CREATE INDEX index_scheduled_sla_notifications_on_send_at ON scheduled_sla_notifications(send_at);
CREATE INDEX index_scheduled_sla_notifications_on_processed_at ON scheduled_sla_notifications(processed_at);
CREATE INDEX index_scheduled_sla_notifications_on_tenant_id ON scheduled_sla_notifications (tenant_id);
