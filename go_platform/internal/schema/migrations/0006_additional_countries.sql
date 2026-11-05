-- Insert additional Middle East countries with their subdomains
INSERT INTO core_country (code, name_en, name_ar, currency, currency_label_ar, locale, enabled, hostname, timezone, default_locale, enabled_public_site)
VALUES
    ('SA', 'Saudi Arabia', 'المملكة العربية السعودية', 'SAR', 'ريال', 'ar-SA', true, 'sa.ahsanli.com', 'Asia/Riyadh', 'ar', true),
    ('JO', 'Jordan', 'الأردن', 'JOD', 'دينار', 'ar-JO', true, 'jo.ahsanli.com', 'Asia/Amman', 'ar', true),
    ('AE', 'United Arab Emirates', 'الإمارات', 'AED', 'درهم', 'ar-AE', true, 'ae.ahsanli.com', 'Asia/Dubai', 'ar', true),
    ('KW', 'Kuwait', 'الكويت', 'KWD', 'دينار', 'ar-KW', true, 'kw.ahsanli.com', 'Asia/Kuwait', 'ar', true),
    ('OM', 'Oman', 'عُمان', 'OMR', 'ريال', 'ar-OM', true, 'om.ahsanli.com', 'Asia/Muscat', 'ar', true),
    ('BH', 'Bahrain', 'البحرين', 'BHD', 'دينار', 'ar-BH', true, 'bh.ahsanli.com', 'Asia/Bahrain', 'ar', true),
    ('QA', 'Qatar', 'قطر', 'QAR', 'ريال', 'ar-QA', true, 'qa.ahsanli.com', 'Asia/Qatar', 'ar', true),
    ('IQ', 'Iraq', 'العراق', 'IQD', 'دينار', 'ar-IQ', true, 'iq.ahsanli.com', 'Asia/Baghdad', 'ar', true)
ON CONFLICT (code) DO UPDATE SET
    name_en = EXCLUDED.name_en,
    name_ar = EXCLUDED.name_ar,
    currency = EXCLUDED.currency,
    currency_label_ar = EXCLUDED.currency_label_ar,
    locale = EXCLUDED.locale,
    enabled = EXCLUDED.enabled,
    hostname = EXCLUDED.hostname,
    timezone = EXCLUDED.timezone,
    default_locale = EXCLUDED.default_locale,
    enabled_public_site = EXCLUDED.enabled_public_site;

-- Create default core_dealrule for each newly added country
INSERT INTO core_dealrule (name, country_id, historical_percent, cross_store_percent, cooldown_hours, freshness_hours, version, enabled, cash_floor_10000_plus, cash_floor_under_10000, cash_floor_under_2000, cash_floor_under_500)
SELECT
    'default',
    c.id,
    10.00,
    5.00,
    24,
    2,
    1,
    true,
    300.00,
    150.00,
    50.00,
    10.00
FROM core_country c
WHERE c.code IN ('SA', 'JO', 'AE', 'KW', 'OM', 'BH', 'QA', 'IQ')
AND NOT EXISTS (
    SELECT 1 FROM core_dealrule r WHERE r.country_id = c.id
);
