-- Country public hostname, timezone, and default locale for multi-country launch.
ALTER TABLE core_country
    ADD COLUMN IF NOT EXISTS hostname VARCHAR(253) UNIQUE,
    ADD COLUMN IF NOT EXISTS timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
    ADD COLUMN IF NOT EXISTS default_locale VARCHAR(5) NOT NULL DEFAULT 'ar',
    ADD COLUMN IF NOT EXISTS enabled_public_site BOOLEAN NOT NULL DEFAULT FALSE;

-- Backfill Egypt from existing values; operators must set other countries explicitly.
UPDATE core_country
SET hostname = 'eg.ahsanli.com',
    timezone = 'Africa/Cairo',
    default_locale = 'ar',
    enabled_public_site = TRUE
WHERE code = 'EG';
