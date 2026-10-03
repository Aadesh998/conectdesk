-- AI agent: tools, assistants, their events, copilot chats and FAQ suggestions.

DROP TABLE IF EXISTS ai_tools CASCADE;
CREATE TABLE ai_tools (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	url TEXT NOT NULL,
	method TEXT NOT NULL DEFAULT 'POST',
	auth JSONB NOT NULL DEFAULT '{}',
	parameters JSONB NOT NULL DEFAULT '{}',
	enabled BOOLEAN NOT NULL DEFAULT true,
	requires_verification BOOLEAN NOT NULL DEFAULT true,
	CONSTRAINT constraint_ai_tools_on_name CHECK (name ~ '^[a-zA-Z0-9_-]+$' AND length(name) <= 64),
	CONSTRAINT uniq_ai_tools_name UNIQUE (tenant_id, "name")
);
CREATE INDEX index_ai_tools_on_tenant_id ON ai_tools (tenant_id);

DROP TABLE IF EXISTS ai_assistants CASCADE;
CREATE TABLE ai_assistants (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
	description TEXT NOT NULL DEFAULT '',
	instructions TEXT NOT NULL DEFAULT '',
	guardrails TEXT NOT NULL DEFAULT '',
	expectation TEXT NOT NULL DEFAULT '',
	tone TEXT NOT NULL DEFAULT 'professional',
	response_length TEXT NOT NULL DEFAULT 'balanced',
	max_turns INTEGER NOT NULL DEFAULT 6,
	fallback_team_id INTEGER NULL REFERENCES teams(id) ON DELETE SET NULL,
	handoff_enabled BOOLEAN NOT NULL DEFAULT true,
	languages TEXT[] NOT NULL DEFAULT '{}',
	enabled BOOLEAN NOT NULL DEFAULT true,
	CONSTRAINT constraint_ai_assistants_on_tone CHECK (tone IN ('friendly', 'professional', 'neutral', 'casual')),
	CONSTRAINT constraint_ai_assistants_on_response_length CHECK (response_length IN ('concise', 'balanced', 'detailed')),
	CONSTRAINT constraint_ai_assistants_on_max_turns CHECK (max_turns > 0 AND max_turns <= 20)
);
CREATE INDEX index_ai_assistants_on_user_id ON ai_assistants(user_id);
CREATE INDEX index_ai_assistants_on_tenant_id ON ai_assistants (tenant_id);

DROP TABLE IF EXISTS ai_assistant_tools CASCADE;
CREATE TABLE ai_assistant_tools (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	assistant_id INTEGER NOT NULL REFERENCES ai_assistants(id) ON DELETE CASCADE,
	tool_id INTEGER NOT NULL REFERENCES ai_tools(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX index_unique_ai_assistant_tools ON ai_assistant_tools(assistant_id, tool_id);
CREATE INDEX index_ai_assistant_tools_on_assistant_id ON ai_assistant_tools(assistant_id);
CREATE INDEX index_ai_assistant_tools_on_tenant_id ON ai_assistant_tools (tenant_id);

DROP TABLE IF EXISTS ai_agent_events CASCADE;
CREATE TABLE ai_agent_events (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	assistant_id INTEGER NOT NULL REFERENCES ai_assistants(id) ON DELETE CASCADE,
	conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	type TEXT NOT NULL,
	CONSTRAINT constraint_ai_agent_events_on_type CHECK (type IN ('handoff', 'resolve'))
);
CREATE INDEX index_ai_agent_events_on_assistant_type_created ON ai_agent_events(assistant_id, type, created_at);
CREATE INDEX index_ai_agent_events_on_conversation_id ON ai_agent_events(conversation_id);
CREATE INDEX index_ai_agent_events_on_tenant_id ON ai_agent_events (tenant_id);

DROP TABLE IF EXISTS copilot_messages CASCADE;
CREATE TABLE copilot_messages (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	"role" TEXT NOT NULL,
	content TEXT NOT NULL,
	CONSTRAINT constraint_copilot_messages_on_role CHECK ("role" IN ('user', 'assistant'))
);
CREATE INDEX index_copilot_messages_on_conversation_user ON copilot_messages(conversation_id, user_id, id);
CREATE INDEX index_copilot_messages_on_tenant_id ON copilot_messages (tenant_id);

-- An agent's copilot chat on a report, such as the SLA dashboard, rather than a conversation.
DROP TABLE IF EXISTS report_copilot_messages CASCADE;
CREATE TABLE report_copilot_messages (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	scope TEXT NOT NULL,
	"role" TEXT NOT NULL,
	content TEXT NOT NULL,
	CONSTRAINT constraint_report_copilot_messages_on_scope CHECK (scope IN ('sla')),
	CONSTRAINT constraint_report_copilot_messages_on_role CHECK ("role" IN ('user', 'assistant'))
);
CREATE INDEX index_report_copilot_messages_on_user_scope ON report_copilot_messages (tenant_id, user_id, scope, id);
CREATE INDEX index_report_copilot_messages_on_tenant_id ON report_copilot_messages (tenant_id);

DROP TABLE IF EXISTS ai_faq_suggestions CASCADE;
CREATE TABLE ai_faq_suggestions (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	question TEXT NOT NULL,
	answer TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	reviewed_by_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
	reviewed_at TIMESTAMPTZ NULL,
	CONSTRAINT constraint_ai_faq_suggestions_on_status CHECK (status IN ('pending', 'approved', 'rejected'))
);
CREATE INDEX index_ai_faq_suggestions_on_status_created ON ai_faq_suggestions(status, created_at);
CREATE INDEX index_ai_faq_suggestions_on_conversation_id ON ai_faq_suggestions(conversation_id);
CREATE INDEX index_ai_faq_suggestions_on_tenant_id ON ai_faq_suggestions (tenant_id);
