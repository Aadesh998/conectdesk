package migrations

import (
	"fmt"
	"slices"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// TenantScopedTables lists every table that holds tenant-owned rows. Each one
// carries a tenant_id column pointing at tenants(id). A new table must be added
// here and to schema/, and the schema test checks the two agree.
var TenantScopedTables = []string{
	"sla_policies", "business_hours", "inboxes", "teams", "roles", "users", "user_roles",
	"conversation_statuses", "conversation_priorities", "conversations", "conversation_messages",
	"automation_rules", "conversation_drafts", "macros", "conversation_participants",
	"conversation_mentions", "conversation_last_seen", "media", "oidc", "settings", "tags",
	"team_members", "templates", "conversation_tags", "csat_responses", "views", "applied_slas",
	"sla_events", "scheduled_sla_notifications", "ai_providers", "ai_prompts", "ai_knowledge_base",
	"embeddings", "help_centers", "article_collections", "help_articles", "help_article_feedback",
	"help_search_queries", "ai_tools", "ai_assistants", "ai_assistant_tools", "ai_agent_events",
	"copilot_messages", "ai_faq_suggestions", "custom_attribute_definitions", "contact_notes",
	"activity_logs", "webhooks", "context_links", "user_notifications",
}

// PlatformTables belong to the installation rather than to a tenant, and have
// no tenant_id.
var PlatformTables = []string{"tenants"}

// V2_10_0 introduces tenants. Existing rows are assigned to tenant 1, the
// default tenant, and the column default keeps single-tenant queries working
// until every query passes the tenant explicitly.
func V2_10_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'tenant_status') THEN
				CREATE TYPE "tenant_status" AS ENUM ('active', 'suspended');
			END IF;
		END$$;

		CREATE TABLE IF NOT EXISTS tenants (
			id SERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW() NOT NULL,
			slug TEXT NOT NULL,
			"name" TEXT NOT NULL,
			custom_domain TEXT NULL,
			status tenant_status NOT NULL DEFAULT 'active',
			"plan" TEXT NOT NULL DEFAULT 'free',
			meta JSONB NOT NULL DEFAULT '{}'::jsonb,
			CONSTRAINT constraint_tenants_on_slug CHECK (slug ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$'),
			CONSTRAINT constraint_tenants_on_name CHECK (length("name") <= 140)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS index_tenants_on_slug ON tenants (slug);
		CREATE UNIQUE INDEX IF NOT EXISTS index_tenants_on_custom_domain ON tenants (LOWER(custom_domain)) WHERE custom_domain IS NOT NULL;

		INSERT INTO tenants (id, slug, "name") VALUES (1, 'default', 'Default') ON CONFLICT (id) DO NOTHING;
		SELECT setval('tenants_id_seq', GREATEST((SELECT MAX(id) FROM tenants), 1));
	`); err != nil {
		return err
	}

	for _, t := range TenantScopedTables {
		if _, err := tx.Exec(fmt.Sprintf(`
			ALTER TABLE %[1]s ADD COLUMN IF NOT EXISTS tenant_id INT NOT NULL DEFAULT 1
				REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE;
			CREATE INDEX IF NOT EXISTS index_%[1]s_on_tenant_id ON %[1]s (tenant_id);
		`, t)); err != nil {
			return fmt.Errorf("adding tenant_id to %s: %w", t, err)
		}
	}

	return tx.Commit()
}

// AllTenantTables lists every tenant table, including those added after V2_10_0.
func AllTenantTables() []string {
	return slices.Concat(TenantScopedTables, TenantScopedTablesV2_14_0, TenantScopedTablesV2_16_0, TenantScopedTablesV2_20_0, TenantScopedTablesV2_27_0, TenantScopedTablesV2_31_0, TenantScopedTablesV2_32_0, TenantScopedTablesV2_33_0, TenantScopedTablesV2_35_0)
}
