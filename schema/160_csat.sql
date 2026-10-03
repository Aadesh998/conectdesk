-- CSAT survey responses.

DROP TABLE IF EXISTS csat_responses CASCADE;
CREATE TABLE csat_responses (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
	uuid UUID DEFAULT gen_random_uuid() NOT NULL UNIQUE,

	-- Cascade deletes when conversation is deleted.
    conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,

    rating INT DEFAULT 0 NOT NULL,
    feedback TEXT NULL,
    meta JSONB DEFAULT '{}' NOT NULL,
    response_timestamp TIMESTAMPTZ NULL,
    CONSTRAINT constraint_csat_responses_on_rating CHECK (rating >= 0 AND rating <= 5),
    CONSTRAINT constraint_csat_responses_on_feedback CHECK (length(feedback) <= 1000)
);
CREATE INDEX index_csat_responses_on_uuid ON csat_responses(uuid);
CREATE INDEX index_csat_responses_on_conversation_id ON csat_responses(conversation_id);
CREATE INDEX index_csat_responses_on_tenant_id ON csat_responses (tenant_id);
