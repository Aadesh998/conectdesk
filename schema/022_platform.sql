-- The platform admin dashboard's tables. Platform admins run the installation
-- and are not users of any tenant; the tenants they act on are "workspaces"
-- here, and these tables have no tenant_id.

DROP TABLE IF EXISTS platform_admins CASCADE;
CREATE TABLE platform_admins (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	email TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	last_login_at TIMESTAMPTZ NULL,
	-- Two-factor login, as for agents: encrypted secret, when it was switched
	-- on, hashed recovery codes and the last accepted time step.
	totp_secret TEXT NULL,
	totp_enabled_at TIMESTAMPTZ NULL,
	totp_recovery_codes TEXT[] NOT NULL DEFAULT '{}',
	totp_last_step BIGINT NOT NULL DEFAULT 0,
	CONSTRAINT constraint_platform_admins_on_email CHECK (email = LOWER(email) AND length(email) <= 254)
);
CREATE UNIQUE INDEX index_platform_admins_on_email ON platform_admins (email);

DROP TABLE IF EXISTS platform_sessions CASCADE;
CREATE TABLE platform_sessions (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	admin_id INT NOT NULL REFERENCES platform_admins(id) ON DELETE CASCADE,
	-- SHA-256 of the session cookie; the cookie itself is never stored.
	token_hash TEXT NOT NULL,
	ip TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX index_platform_sessions_on_token_hash ON platform_sessions (token_hash);

-- Everything a platform admin does. Rows outlive the admin and the workspace,
-- so the audit keeps their email and slug.
DROP TABLE IF EXISTS platform_audit_log CASCADE;
CREATE TABLE platform_audit_log (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	admin_id INT NULL REFERENCES platform_admins(id) ON DELETE SET NULL,
	admin_email TEXT NOT NULL,
	action TEXT NOT NULL,
	workspace_id INT NULL REFERENCES tenants(id) ON DELETE SET NULL,
	workspace_slug TEXT NOT NULL DEFAULT '',
	details JSONB NOT NULL DEFAULT '{}'::jsonb,
	ip TEXT NOT NULL DEFAULT ''
);
CREATE INDEX index_platform_audit_log_on_workspace_id ON platform_audit_log (workspace_id, id DESC);
CREATE INDEX index_platform_audit_log_on_created_at ON platform_audit_log (created_at DESC);

-- One-time links that log a platform admin into a workspace as one of its agents.
DROP TABLE IF EXISTS platform_impersonations CASCADE;
CREATE TABLE platform_impersonations (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	used_at TIMESTAMPTZ NULL,
	admin_id INT NOT NULL REFERENCES platform_admins(id) ON DELETE CASCADE,
	workspace_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
	user_id INT NOT NULL,
	token_hash TEXT NOT NULL
);
CREATE UNIQUE INDEX index_platform_impersonations_on_token_hash ON platform_impersonations (token_hash);

-- Settings of the installation, set from the platform dashboard. "mail" is the
-- platform's own mailer: it sends signup verification and the email of every
-- tenant without a mailer of its own. Its keys match a tenant's
-- notification.email.* settings, without the prefix; the password is encrypted.
DROP TABLE IF EXISTS platform_settings CASCADE;
CREATE TABLE platform_settings (
	key TEXT PRIMARY KEY,
	value JSONB NOT NULL DEFAULT '{}'::jsonb,
	updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- People who may read the internal documentation on docs.<base_domain>, and
-- nothing else: they aren't platform admins nor users of any workspace.
-- Platform admins add and remove them.
DROP TABLE IF EXISTS platform_docs_viewers CASCADE;
CREATE TABLE platform_docs_viewers (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	email TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	last_login_at TIMESTAMPTZ NULL,
	CONSTRAINT constraint_platform_docs_viewers_on_email CHECK (email = LOWER(email) AND length(email) <= 254)
);
CREATE UNIQUE INDEX index_platform_docs_viewers_on_email ON platform_docs_viewers (email);

DROP TABLE IF EXISTS platform_docs_sessions CASCADE;
CREATE TABLE platform_docs_sessions (
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	viewer_id INT NOT NULL REFERENCES platform_docs_viewers(id) ON DELETE CASCADE,
	-- SHA-256 of the session cookie; the cookie itself is never stored.
	token_hash TEXT NOT NULL,
	ip TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX index_platform_docs_sessions_on_token_hash ON platform_docs_sessions (token_hash);
