-- Curated public category taxonomy. Products are classified only when source data is certain.
CREATE TABLE IF NOT EXISTS core_category (
    id SERIAL PRIMARY KEY,
    code VARCHAR(64) UNIQUE NOT NULL,
    name_ar VARCHAR(160) NOT NULL,
    name_en VARCHAR(160) NOT NULL,
    parent_code VARCHAR(64) REFERENCES core_category(code) ON DELETE SET NULL,
    position INTEGER NOT NULL DEFAULT 0
);

-- Optional classification of a listing. A listing may remain unclassified.
ALTER TABLE core_listing
    ADD COLUMN IF NOT EXISTS category_id INTEGER REFERENCES core_category(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS listing_category_idx ON core_listing(category_id);

-- Seed Egypt categories supported by existing source data. Add others after verified mapping.
INSERT INTO core_category (code, name_ar, name_en, position) VALUES
    ('phones', 'موبايلات', 'Mobile Phones', 10),
    ('electronics', 'إلكترونيات', 'Electronics', 20),
    ('home', 'منزل ومطبخ', 'Home & Kitchen', 30),
    ('fashion', 'أزياء', 'Fashion', 40)
ON CONFLICT (code) DO NOTHING;
