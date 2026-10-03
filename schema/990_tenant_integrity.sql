-- Tenant-scoped foreign keys. Every reference between tenant tables includes
-- tenant_id, so a row can only point at a row of its own tenant. Platform
-- tables have no tenant_id, and their references are left alone.
DO $$
DECLARE
	r RECORD;
BEGIN
	-- Referenced tables need a unique (tenant_id, id) key.
	FOR r IN
		SELECT DISTINCT pc.relname AS parent
		FROM pg_constraint con
		JOIN pg_class pc ON pc.oid = con.confrelid
		JOIN pg_namespace n ON n.oid = pc.relnamespace
		WHERE con.contype = 'f' AND n.nspname = 'public' AND pc.relname <> 'tenants'
		  AND EXISTS (SELECT 1 FROM pg_attribute ta WHERE ta.attrelid = con.conrelid AND ta.attname = 'tenant_id' AND NOT ta.attisdropped)
	LOOP
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'uniq_' || r.parent || '_tenant_id_id') THEN
			EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I UNIQUE (tenant_id, id)', r.parent, 'uniq_' || r.parent || '_tenant_id_id');
		END IF;
	END LOOP;

	-- Replace each single-column reference to (id) with a composite one. SET NULL
	-- clears only the referencing column, never tenant_id. RESTRICT becomes NO
	-- ACTION so deleting a tenant can cascade through rows that reference each other.
	FOR r IN
		SELECT con.conname, cc.relname AS child, a.attname AS col, pc.relname AS parent, con.confdeltype, con.confupdtype
		FROM pg_constraint con
		JOIN pg_class cc ON cc.oid = con.conrelid
		JOIN pg_class pc ON pc.oid = con.confrelid
		JOIN pg_namespace n ON n.oid = cc.relnamespace
		JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = con.conkey[1]
		JOIN pg_attribute pa ON pa.attrelid = con.confrelid AND pa.attnum = con.confkey[1]
		WHERE con.contype = 'f' AND n.nspname = 'public' AND pc.relname <> 'tenants'
		  AND array_length(con.conkey, 1) = 1 AND pa.attname = 'id'
		  AND EXISTS (SELECT 1 FROM pg_attribute ta WHERE ta.attrelid = con.conrelid AND ta.attname = 'tenant_id' AND NOT ta.attisdropped)
	LOOP
		EXECUTE format('ALTER TABLE %I DROP CONSTRAINT %I', r.child, r.conname);
		EXECUTE format(
			'ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (tenant_id, %I) REFERENCES %I (tenant_id, id) ON DELETE %s ON UPDATE %s',
			r.child, left('fk_' || r.child || '_' || r.col, 63), r.col, r.parent,
			CASE r.confdeltype WHEN 'c' THEN 'CASCADE' WHEN 'n' THEN format('SET NULL (%I)', r.col) ELSE 'NO ACTION' END,
			CASE r.confupdtype WHEN 'c' THEN 'CASCADE' ELSE 'NO ACTION' END);
	END LOOP;
END$$;

-- tenant_id has no default, so an insert that doesn't name its tenant fails
-- instead of landing in the default tenant. The seed rows above belong to the
-- default tenant. Settings keeps the default for migration bookkeeping.
DO $$
DECLARE
	r RECORD;
BEGIN
	FOR r IN
		SELECT table_name FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name = 'tenant_id' AND table_name <> 'settings'
	LOOP
		EXECUTE format('ALTER TABLE %I ALTER COLUMN tenant_id DROP DEFAULT', r.table_name);
	END LOOP;
END$$;
