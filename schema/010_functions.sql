-- Functions.

-- Function to generate the next conversation reference number of a tenant, with optional prefix.
-- Each tenant counts from 100 on its own, so numbers don't reveal other tenants' volume.
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

-- Function to pick the text search configuration for a help article locale.
CREATE OR REPLACE FUNCTION help_article_search_config(locale TEXT)
RETURNS regconfig AS $$
    SELECT CASE split_part(locale, '-', 1)
        WHEN 'ar' THEN 'arabic'
        WHEN 'da' THEN 'danish'
        WHEN 'nl' THEN 'dutch'
        WHEN 'en' THEN 'english'
        WHEN 'fi' THEN 'finnish'
        WHEN 'fr' THEN 'french'
        WHEN 'de' THEN 'german'
        WHEN 'el' THEN 'greek'
        WHEN 'hu' THEN 'hungarian'
        WHEN 'id' THEN 'indonesian'
        WHEN 'ga' THEN 'irish'
        WHEN 'it' THEN 'italian'
        WHEN 'lt' THEN 'lithuanian'
        WHEN 'ne' THEN 'nepali'
        WHEN 'no' THEN 'norwegian'
        WHEN 'pt' THEN 'portuguese'
        WHEN 'ro' THEN 'romanian'
        WHEN 'ru' THEN 'russian'
        WHEN 'es' THEN 'spanish'
        WHEN 'sv' THEN 'swedish'
        WHEN 'ta' THEN 'tamil'
        WHEN 'tr' THEN 'turkish'
        ELSE 'simple'
    END::regconfig;
$$ LANGUAGE sql IMMUTABLE;
