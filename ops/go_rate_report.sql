-- Read-only hourly Amazon offer-fetch report. Run as the PostgreSQL OS user:
-- psql -d botafflite -f ops/go_rate_report.sql
-- Excludes the current incomplete hour. A success means a directly verified
-- price and seller; unavailable products are reported separately.
WITH hourly AS (
  SELECT date_trunc('hour', a.started_at) AS hour_utc,
         count(*) AS attempts,
         count(*) FILTER (WHERE a.result = 'success') AS valid_offers,
         count(*) FILTER (WHERE a.result = 'unavailable') AS unavailable,
         count(*) FILTER (WHERE a.error_class = 'blocked') AS blocked,
         count(*) FILTER (WHERE a.error_class = 'parser_error') AS parser_errors,
         count(*) FILTER (WHERE a.error_class = 'network_or_parser_error') AS network_errors,
         round(avg(a.elapsed_ms)) AS mean_elapsed_ms
  FROM core_fetchattempt a
  JOIN core_fetchjob j ON j.id = a.job_id
  JOIN core_listing l ON l.id = j.listing_id
  JOIN core_retailer r ON r.id = l.retailer_id
  WHERE r.code = 'amazon_eg'
    AND a.started_at >= now() - interval '24 hours'
    AND a.started_at < date_trunc('hour', now())
  GROUP BY 1
)
SELECT hour_utc, attempts, valid_offers, unavailable, blocked,
       parser_errors, network_errors, mean_elapsed_ms
FROM hourly
ORDER BY hour_utc DESC;
