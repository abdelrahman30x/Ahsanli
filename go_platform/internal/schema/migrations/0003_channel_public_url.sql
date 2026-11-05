-- Public URL and message locale for each channel.
ALTER TABLE core_channel
    ADD COLUMN IF NOT EXISTS public_url VARCHAR(500),
    ADD COLUMN IF NOT EXISTS message_locale VARCHAR(5) NOT NULL DEFAULT 'ar';

-- Only public @handles can form a valid t.me URL; numeric chat IDs cannot.
UPDATE core_channel
SET public_url = 'https://t.me/' || substring(destination_id FROM 2)
WHERE public_url IS NULL AND platform = 'telegram' AND destination_id ~ '^@[A-Za-z][A-Za-z0-9_]{4,31}$';
