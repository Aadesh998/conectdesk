package migrations

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// TenantForeignKeysSQL turns every single-column reference between tenant
// tables into a composite (tenant_id, column) reference, so a row can only point
// at a row of its own tenant. It is idempotent and schema/ runs the same
// block, which the schema test checks.
const TenantForeignKeysSQL = `DO $$
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
END$$;`

// tenantUnique is a uniqueness rule that becomes per-tenant.
type tenantUnique struct {
	table   string
	oldCols []string
	name    string
	cols    string
}

// tenantUniqueIndex is a partial unique index that becomes per-tenant.
type tenantUniqueIndex struct {
	name, def string
}

var (
	tenantUniques = []tenantUnique{
		{"roles", []string{"name"}, "uniq_roles_name", `tenant_id, "name"`},
		{"conversation_statuses", []string{"name"}, "uniq_conversation_statuses_name", `tenant_id, "name"`},
		{"conversation_priorities", []string{"name"}, "uniq_conversation_priorities_name", `tenant_id, "name"`},
		{"conversations", []string{"reference_number"}, "uniq_conversations_reference_number", `tenant_id, reference_number`},
		{"settings", []string{"key"}, "uniq_settings_key", `tenant_id, "key"`},
		{"tags", []string{"name"}, "uniq_tags_name", `tenant_id, "name"`},
		{"ai_providers", []string{"name"}, "uniq_ai_providers_name", `tenant_id, "name"`},
		{"ai_prompts", []string{"key"}, "uniq_ai_prompts_key", `tenant_id, "key"`},
		{"help_centers", []string{"slug"}, "uniq_help_centers_slug", `tenant_id, slug`},
		{"ai_tools", []string{"name"}, "uniq_ai_tools_name", `tenant_id, "name"`},
		{"teams", []string{"name"}, "constraint_teams_on_name_unique", `tenant_id, "name"`},
		{"custom_attribute_definitions", []string{"key", "applies_to"}, "constraint_custom_attribute_definitions_key_applies_to_unique", `tenant_id, key, applies_to`},
	}

	tenantUniqueIndexes = []tenantUniqueIndex{
		{"index_unique_users_on_email_when_type_is_agent", `ON users (tenant_id, email) WHERE type = 'agent' AND deleted_at IS NULL`},
		{"index_unique_users_on_ext_id_when_type_is_contact", `ON users (tenant_id, external_user_id) WHERE type = 'contact' AND deleted_at IS NULL AND external_user_id IS NOT NULL`},
		{"index_unique_users_on_email_when_no_ext_id_contact", `ON users (tenant_id, email) WHERE type = 'contact' AND deleted_at IS NULL AND external_user_id IS NULL`},
		{"index_unique_templates_on_is_default_when_is_default_is_true", `ON templates (tenant_id, is_default) WHERE is_default = true`},
		{"index_unique_ai_providers_on_is_default_when_is_default_is_true", `ON ai_providers (tenant_id, is_default) WHERE is_default = true`},
		{"index_unique_ai_providers_on_type", `ON ai_providers (tenant_id, type)`},
		{"index_conversation_messages_on_source_id", `ON conversation_messages (tenant_id, source_id) WHERE source_id IS NOT NULL AND source_id <> ''`},
	}
)

// V2_11_0 enforces tenant integrity in the database: names, emails and other
// unique values become unique per tenant, references can't cross tenants,
// conversation reference numbers are counted per tenant, and tenant_id loses its
// default so an insert that doesn't name its tenant fails instead of landing in
// the default tenant.
func V2_11_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	var ver int
	if err := db.Get(&ver, `SELECT current_setting('server_version_num')::int`); err != nil {
		return err
	}
	if ver < 150000 {
		return fmt.Errorf("PostgreSQL 15 or newer is required, found %d", ver)
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := migrateReferenceNumbers(tx); err != nil {
		return fmt.Errorf("reference numbers: %w", err)
	}

	for _, u := range tenantUniques {
		if err := makeUniquePerTenant(tx, u); err != nil {
			return fmt.Errorf("unique %s: %w", u.name, err)
		}
	}
	for _, idx := range tenantUniqueIndexes {
		if _, err := tx.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %[1]s; CREATE UNIQUE INDEX %[1]s %[2]s`, idx.name, idx.def)); err != nil {
			return fmt.Errorf("index %s: %w", idx.name, err)
		}
	}

	if _, err := tx.Exec(TenantForeignKeysSQL); err != nil {
		return fmt.Errorf("foreign keys: %w", err)
	}

	// Settings keeps its default: migration bookkeeping writes the default
	// tenant's `migrations` key from code that predates tenants.
	for _, t := range TenantScopedTables {
		if t == "settings" {
			continue
		}
		if _, err := tx.Exec(`ALTER TABLE ` + t + ` ALTER COLUMN tenant_id DROP DEFAULT`); err != nil {
			return fmt.Errorf("dropping tenant_id default on %s: %w", t, err)
		}
	}

	return tx.Commit()
}

// migrateReferenceNumbers moves conversation numbering from the global
// sequence to a per-tenant counter, continuing the default tenant where the
// sequence left off.
func migrateReferenceNumbers(tx *sqlx.Tx) error {
	if _, err := tx.Exec(`ALTER TABLE tenants ADD COLUMN IF NOT EXISTS next_reference_number BIGINT NOT NULL DEFAULT 100`); err != nil {
		return err
	}

	var seqExists bool
	if err := tx.Get(&seqExists, `SELECT to_regclass('conversation_reference_number_sequence') IS NOT NULL`); err != nil {
		return err
	}
	if seqExists {
		if _, err := tx.Exec(`
			UPDATE tenants SET next_reference_number = (
				SELECT CASE WHEN is_called THEN last_value + 1 ELSE last_value END
				FROM conversation_reference_number_sequence
			) WHERE id = 1`); err != nil {
			return err
		}
	}

	_, err := tx.Exec(`
		ALTER TABLE conversations ALTER COLUMN reference_number DROP DEFAULT;
		DROP FUNCTION IF EXISTS generate_reference_number(TEXT);
		DROP SEQUENCE IF EXISTS conversation_reference_number_sequence;

		CREATE OR REPLACE FUNCTION generate_reference_number(p_tenant_id INT, prefix TEXT)
		RETURNS TEXT AS $$
		DECLARE
			n BIGINT;
		BEGIN
			UPDATE tenants SET next_reference_number = next_reference_number + 1
			WHERE id = p_tenant_id
			RETURNING next_reference_number - 1 INTO n;
			IF n IS NULL THEN
				RAISE EXCEPTION 'tenant % does not exist', p_tenant_id;
			END IF;
			RETURN prefix || n;
		END;
		$$ LANGUAGE plpgsql;
	`)
	return err
}

// makeUniquePerTenant drops the global unique constraint on u.oldCols, whatever
// it is named, and adds the per-tenant one.
func makeUniquePerTenant(tx *sqlx.Tx, u tenantUnique) error {
	var names []string
	if err := tx.Select(&names, `
		SELECT con.conname
		FROM pg_constraint con
		JOIN pg_class c ON c.oid = con.conrelid
		WHERE c.relname = $1 AND con.contype = 'u'
		  AND (SELECT array_agg(a.attname::text ORDER BY a.attname)
		       FROM pg_attribute a
		       WHERE a.attrelid = con.conrelid AND a.attnum = ANY(con.conkey)) = (SELECT array_agg(x ORDER BY x) FROM unnest($2::text[]) x)`,
		u.table, "{"+strings.Join(u.oldCols, ",")+"}"); err != nil {
		return err
	}
	for _, n := range names {
		if _, err := tx.Exec(fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT %q`, u.table, n)); err != nil {
			return err
		}
	}

	var exists bool
	if err := tx.Get(&exists, `SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = $1)`, u.name); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err := tx.Exec(fmt.Sprintf(`ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s)`, u.table, u.name, u.cols))
	return err
}
