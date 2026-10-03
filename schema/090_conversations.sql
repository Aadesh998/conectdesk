-- Conversations and everything attached to one.

DROP TABLE IF EXISTS conversations CASCADE;
CREATE TABLE conversations (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    "uuid" UUID DEFAULT gen_random_uuid() NOT NULL UNIQUE,
	reference_number TEXT NOT NULL,

	-- Cascade deletes when contact is deleted.
    contact_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,

	-- Set to NULL when assigned user or team is deleted.
    assigned_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
    assigned_team_id INT REFERENCES teams(id) ON DELETE SET NULL ON UPDATE CASCADE,

	-- Set to NULL when SLA policy is deleted.
	sla_policy_id INT REFERENCES sla_policies(id) ON DELETE SET NULL ON UPDATE CASCADE,

    -- Cascade deletes when inbox is deleted.
	inbox_id INT REFERENCES inboxes(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,

	-- Restrict delete.
	status_id INT REFERENCES conversation_statuses(id) ON DELETE RESTRICT ON UPDATE CASCADE NOT NULL,
    priority_id INT REFERENCES conversation_priorities(id) ON DELETE RESTRICT ON UPDATE CASCADE,

	meta JSONB DEFAULT '{}'::jsonb NOT NULL,
	custom_attributes JSONB DEFAULT '{}'::jsonb NOT NULL,
	contact_last_seen_at TIMESTAMPTZ DEFAULT NOW(),
    first_reply_at TIMESTAMPTZ NULL,
    last_reply_at TIMESTAMPTZ NULL,
    closed_at TIMESTAMPTZ NULL,
    resolved_at TIMESTAMPTZ NULL,

	"subject" TEXT NULL,
	waiting_since TIMESTAMPTZ NULL,
	last_message_at TIMESTAMPTZ NULL,
	last_message TEXT NULL,
	last_message_sender message_sender_type NULL,
	last_message_sender_id BIGINT REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
	last_interaction TEXT NULL,
	last_interaction_sender message_sender_type NULL,
	last_interaction_sender_id BIGINT REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
	last_interaction_at TIMESTAMPTZ NULL,
	next_sla_deadline_at TIMESTAMPTZ NULL,
	snoozed_until TIMESTAMPTZ NULL,
	last_continuity_email_sent_at TIMESTAMPTZ NULL,
	CONSTRAINT uniq_conversations_reference_number UNIQUE (tenant_id, reference_number)
);
CREATE INDEX index_conversations_on_assigned_user_id ON conversations (assigned_user_id);
CREATE INDEX index_conversations_on_assigned_team_id ON conversations (assigned_team_id);
CREATE INDEX index_conversations_on_snoozed_until ON conversations (snoozed_until);
CREATE INDEX index_conversations_on_contact_id ON conversations (contact_id);
CREATE INDEX index_conversations_on_inbox_id ON conversations (inbox_id);
CREATE INDEX index_conversations_on_status_id ON conversations (status_id);
CREATE INDEX index_conversations_on_priority_id ON conversations (priority_id);
CREATE INDEX index_conversations_on_created_at ON conversations (created_at);
CREATE INDEX index_conversations_on_last_message_at ON conversations (last_message_at);
CREATE INDEX index_conversations_on_last_interaction_at ON conversations (last_interaction_at);
CREATE INDEX index_conversations_on_next_sla_deadline_at ON conversations (next_sla_deadline_at);
CREATE INDEX index_conversations_on_waiting_since ON conversations (waiting_since);
CREATE INDEX index_conversations_on_last_continuity_email_sent_at ON conversations (last_continuity_email_sent_at);
CREATE INDEX index_conversations_on_tenant_id ON conversations (tenant_id);

DROP TABLE IF EXISTS conversation_messages CASCADE;
CREATE TABLE conversation_messages (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    "uuid" UUID DEFAULT gen_random_uuid() NOT NULL UNIQUE,
    "type" message_type NOT NULL,
    status message_status NOT NULL,
    private BOOL DEFAULT FALSE NOT NULL,
    conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
    content_type content_type NULL,
    "content" TEXT NULL,
	text_content TEXT NULL,
    source_id TEXT NULL,
 	sender_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
    sender_type message_sender_type NOT NULL,
    meta JSONB DEFAULT '{}'::JSONB NULL
);
CREATE INDEX index_trgm_conversation_messages_on_text_content ON conversation_messages USING GIN (text_content gin_trgm_ops);
CREATE INDEX index_conversation_messages_on_conversation_id ON conversation_messages (conversation_id);
CREATE INDEX index_conversation_messages_on_created_at ON conversation_messages (created_at);
CREATE UNIQUE INDEX index_conversation_messages_on_source_id ON conversation_messages (tenant_id, source_id) WHERE source_id IS NOT NULL AND source_id <> '';
CREATE INDEX index_conversation_messages_on_status ON conversation_messages (status);
CREATE INDEX index_conversation_messages_on_conversation_id_and_created_at ON conversation_messages (conversation_id, created_at);
CREATE INDEX index_conversation_messages_on_tenant_id ON conversation_messages (tenant_id);

DROP TABLE IF EXISTS conversation_drafts CASCADE;
CREATE TABLE conversation_drafts (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
    type TEXT NOT NULL DEFAULT 'reply',
    content TEXT NOT NULL,
	meta JSONB DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT constraint_conversation_drafts_on_type CHECK (type IN ('reply', 'private_note'))
);
CREATE UNIQUE INDEX index_uniq_conversation_drafts_on_conversation_id_and_user_id_and_type ON conversation_drafts (conversation_id, user_id, type);
CREATE INDEX index_conversation_drafts_on_tenant_id ON conversation_drafts (tenant_id);

DROP TABLE IF EXISTS conversation_participants CASCADE;
CREATE TABLE conversation_participants (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	-- Cascade deletes when user or conversation is deleted.
	user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL
);
CREATE UNIQUE INDEX index_unique_conversation_participants_on_conversation_id_and_user_id ON conversation_participants (conversation_id, user_id);
CREATE INDEX index_conversation_participants_on_tenant_id ON conversation_participants (tenant_id);

DROP TABLE IF EXISTS conversation_mentions CASCADE;
CREATE TABLE conversation_mentions (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	message_id BIGINT REFERENCES conversation_messages(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	mentioned_user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE,
	mentioned_team_id INT REFERENCES teams(id) ON DELETE CASCADE ON UPDATE CASCADE,
	mentioned_by_user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	CONSTRAINT constraint_mention_target CHECK (
		(mentioned_user_id IS NOT NULL AND mentioned_team_id IS NULL) OR
		(mentioned_user_id IS NULL AND mentioned_team_id IS NOT NULL)
	)
);
CREATE INDEX index_conversation_mentions_on_mentioned_user_id ON conversation_mentions(mentioned_user_id);
CREATE INDEX index_conversation_mentions_on_mentioned_team_id ON conversation_mentions(mentioned_team_id);
CREATE INDEX index_conversation_mentions_on_conversation_id ON conversation_mentions(conversation_id);
CREATE INDEX index_conversation_mentions_on_tenant_id ON conversation_mentions (tenant_id);

DROP TABLE IF EXISTS conversation_last_seen CASCADE;
CREATE TABLE conversation_last_seen (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	last_seen_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);
CREATE UNIQUE INDEX index_unique_conversation_last_seen ON conversation_last_seen (conversation_id, user_id);
CREATE INDEX index_conversation_last_seen_on_tenant_id ON conversation_last_seen (tenant_id);

DROP TABLE IF EXISTS conversation_tags CASCADE;
CREATE TABLE conversation_tags (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	-- Cascade deletes when tag or conversation is deleted.
	tag_id INT REFERENCES tags(id) ON DELETE CASCADE ON UPDATE CASCADE,
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE UNIQUE INDEX index_conversation_tags_on_conversation_id_and_tag_id ON conversation_tags (conversation_id, tag_id);
CREATE INDEX index_conversation_tags_on_tenant_id ON conversation_tags (tenant_id);
