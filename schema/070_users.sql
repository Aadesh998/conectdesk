-- Users: agents, contacts, visitors and AI assistants, and their roles.

DROP TABLE IF EXISTS users CASCADE;
CREATE TABLE users (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    type user_type NOT NULL,
    deleted_at TIMESTAMPTZ NULL,
    enabled BOOL DEFAULT TRUE NOT NULL,
    email TEXT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NULL,
	phone_number_country_code TEXT NULL,
    phone_number TEXT NULL,
    country TEXT NULL,
    "password" VARCHAR(150) NULL,
    avatar_url TEXT NULL,
	custom_attributes JSONB DEFAULT '{}'::jsonb NOT NULL,
	external_user_id TEXT NULL,
    reset_password_token TEXT NULL,
    reset_password_token_expiry TIMESTAMPTZ NULL,
	availability_status user_availability_status DEFAULT 'offline' NOT NULL,
	last_active_at TIMESTAMPTZ NULL,
	last_login_at TIMESTAMPTZ NULL,
	-- API key authentication fields
	api_key TEXT NULL,
	api_secret TEXT NULL,
	-- Two-factor login: the TOTP secret, encrypted; when it was switched on; the
	-- SHA-256 of each unused recovery code; and the last time step a code was
	-- accepted for, so no code works twice.
	totp_secret TEXT NULL,
	totp_enabled_at TIMESTAMPTZ NULL,
	totp_recovery_codes TEXT[] NOT NULL DEFAULT '{}',
	totp_last_step BIGINT NOT NULL DEFAULT 0,
	api_key_last_used_at TIMESTAMPTZ NULL,
	-- The trailing digits of the phone number, which match a caller on the
	-- phone line and a sender on WhatsApp to one contact.
	phone_key TEXT GENERATED ALWAYS AS (NULLIF(RIGHT(REGEXP_REPLACE(COALESCE(phone_number, ''), '[^0-9]', '', 'g'), 10), '')) STORED,
	-- Where a contact is in the admissions pipeline, a key of the leads.stages setting.
	lead_stage TEXT NULL,
	lead_stage_changed_at TIMESTAMPTZ NULL,
    CONSTRAINT constraint_users_on_country CHECK (LENGTH(country) <= 140),
    CONSTRAINT constraint_users_on_phone_number CHECK (LENGTH(phone_number) <= 20),
	CONSTRAINT constraint_users_on_phone_number_country_code CHECK (LENGTH(phone_number_country_code) <= 10),
    CONSTRAINT constraint_users_on_email_length CHECK (LENGTH(email) <= 320),
    CONSTRAINT constraint_users_on_first_name CHECK (LENGTH(first_name) <= 140),
    CONSTRAINT constraint_users_on_last_name CHECK (LENGTH(last_name) <= 140),
	CONSTRAINT constraint_users_on_lead_stage CHECK (LENGTH(lead_stage) <= 50)
);
CREATE INDEX index_tgrm_users_on_email ON users USING GIN (email gin_trgm_ops);
CREATE INDEX index_users_on_api_key ON users(api_key);
CREATE UNIQUE INDEX index_unique_users_on_email_when_type_is_agent
	ON users(tenant_id, email)
	WHERE type = 'agent' AND deleted_at IS NULL;
CREATE UNIQUE INDEX index_unique_users_on_ext_id_when_type_is_contact
	ON users (tenant_id, external_user_id)
	WHERE type = 'contact' AND deleted_at IS NULL AND external_user_id IS NOT NULL;
CREATE UNIQUE INDEX index_unique_users_on_email_when_no_ext_id_contact
	ON users (tenant_id, email)
	WHERE type = 'contact' AND deleted_at IS NULL AND external_user_id IS NULL;
CREATE INDEX index_users_on_tenant_id ON users (tenant_id);
CREATE INDEX index_users_on_phone_key ON users (tenant_id, phone_key)
	WHERE type = 'contact' AND deleted_at IS NULL AND phone_key IS NOT NULL;
CREATE INDEX index_users_on_lead_stage ON users (tenant_id, lead_stage, lead_stage_changed_at DESC)
	WHERE type = 'contact' AND deleted_at IS NULL AND lead_stage IS NOT NULL;

DROP TABLE IF EXISTS user_roles CASCADE;
CREATE TABLE user_roles (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),

	-- Cascade deletes when user or role is deleted, as they are not useful without each other.
	user_id INT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	role_id INT REFERENCES roles(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,

	CONSTRAINT constraint_user_roles_on_user_id_and_role_id_unique UNIQUE (user_id, role_id)
);
CREATE INDEX index_user_roles_on_user_id ON user_roles(user_id);
CREATE INDEX index_user_roles_on_tenant_id ON user_roles (tenant_id);
