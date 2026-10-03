-- Hostnames a tenant serves its workspace or help centers on. A hostname only
-- routes to its tenant once the tenant proves it controls the domain with a
-- DNS TXT record carrying the token, and only one tenant can hold it verified.
DROP TABLE IF EXISTS custom_domains CASCADE;
CREATE TABLE custom_domains (
	id SERIAL PRIMARY KEY,
	tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
	-- Lowercase, without scheme or port.
	hostname TEXT NOT NULL,
	-- Published in DNS, so not a secret.
	token TEXT NOT NULL,
	verified_at TIMESTAMPTZ NULL,
	checked_at TIMESTAMPTZ NULL,
	-- Why the last verification failed, empty when it passed.
	last_error TEXT NOT NULL DEFAULT '',
	-- Checks in a row that found the record gone; at 3 the domain is unverified.
	failed_checks INT NOT NULL DEFAULT 0,
	-- Recorded as verified without DNS (in use before verification existed, or
	-- in single-tenant mode), so never re-checked.
	trusted BOOLEAN NOT NULL DEFAULT false,
	CONSTRAINT constraint_custom_domains_on_hostname CHECK (hostname = LOWER(hostname) AND length(hostname) <= 253),
	CONSTRAINT uniq_custom_domains_tenant_id_id UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX index_custom_domains_on_tenant_id_hostname ON custom_domains (tenant_id, hostname);
CREATE UNIQUE INDEX index_custom_domains_on_verified_hostname ON custom_domains (hostname) WHERE verified_at IS NOT NULL;
