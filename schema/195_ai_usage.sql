-- AI requests each tenant made per month, counted against its plan's
-- ai_requests_per_month. One row per tenant and month.
DROP TABLE IF EXISTS ai_usage CASCADE;
CREATE TABLE ai_usage (
	tenant_id INT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	-- First day of the month.
	"month" DATE NOT NULL,
	requests INT NOT NULL DEFAULT 0,
	PRIMARY KEY (tenant_id, "month")
);
