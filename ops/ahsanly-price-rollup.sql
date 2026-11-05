-- Daily price summary rollup for AHSANLY.
-- Run once per day via ahsanly-price-rollup.service.
INSERT INTO core_dailypricesummary (listing_id, day, min_price, max_price, sample_count)
SELECT listing_id, date_trunc('day', observed_at)::date AS day,
       MIN(price) AS min_price, MAX(price) AS max_price, COUNT(*) AS sample_count
FROM core_offerobservation
WHERE observed_at >= date_trunc('day', now() - interval '1 day')
  AND observed_at < date_trunc('day', now())
GROUP BY listing_id, day
ON CONFLICT (listing_id, day) DO UPDATE SET
  min_price = EXCLUDED.min_price,
  max_price = EXCLUDED.max_price,
  sample_count = EXCLUDED.sample_count;
