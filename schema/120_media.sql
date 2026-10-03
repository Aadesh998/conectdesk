-- Media: uploaded files and attachments.

DROP TABLE IF EXISTS media CASCADE;
CREATE TABLE media (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	"uuid" uuid DEFAULT gen_random_uuid() NOT NULL UNIQUE,
	store "media_store" NOT NULL,
	filename TEXT NOT NULL,
	content_type TEXT NOT NULL,
	content_id TEXT NULL,
	model_id INT NULL,
	model_type TEXT NULL,
	disposition media_disposition NULL,
	"size" INT NULL,
	meta jsonb DEFAULT '{}'::jsonb NOT NULL,
	private BOOLEAN NOT NULL DEFAULT true,
	CONSTRAINT constraint_media_on_filename CHECK (length(filename) <= 1000),
	CONSTRAINT constraint_media_on_content_id CHECK (length(content_id) <= 300)
);
CREATE INDEX index_media_on_model_type_and_model_id ON media(model_type, model_id);
CREATE INDEX index_media_on_content_id ON media(content_id);
CREATE INDEX index_media_on_tenant_id ON media (tenant_id);
