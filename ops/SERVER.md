# AHSANLY server operations

Last verified: 2026-11-05.

## Current state

AHSANLY | أحسنلي is live on the same VPS (`134.122.89.95`) under `/srv/ahsanly`.

| Component | Unit / path | Bind / notes |
| --- | --- | --- |
| Go API | `ahsanly-api.service` | `127.0.0.1:8083` |
| Next.js public site | `ahsanly-web.service` | `127.0.0.1:3100` |
| Amazon price worker | `ahsanly-worker.service` | `--apply --live-candidates` |
| Photo deal pipeline | `ahsanly-pipeline.service` | `--apply --allow-live-channels` |
| Nginx | `/etc/nginx/sites-available/ahsanly` | `ahsanli.com`, `eg.ahsanli.com`, `www.ahsanli.com`, `134.122.89.95` |
| PostgreSQL | database `ahsanly`, role `ahsanly_app` | loopback only |
| Secrets | `/etc/ahsanly/ahsanly.env` | root-only, mode `0600` |
| Captures | `/var/lib/ahsanly/captures` | owned by `botdeals` |
| Backups | `/var/backups/ahsanly/pg` | daily via `ahsanly-db-backup.timer` |
| DB-IP Lite | `/etc/ahsanly/dbip-country-lite.mmdb` | monthly update via `ahsanly-geoip-update.timer` |
| Price rollup | `ahsanly-price-rollup.timer` | SQL-based, no Django dependency |

The BotAfflite Go/Next services are stopped and disabled. The old code remains at `/root/botAfflite` for reference and rollback.

## DNS and TLS

Namecheap A records:
- `@` → `134.122.89.95`
- `eg` → `134.122.89.95`
- `www` → `134.122.89.95` (or CNAME to apex)

Let's Encrypt certificate covers `ahsanli.com`, `eg.ahsanli.com`, `www.ahsanli.com`. Renewal is handled by the system `certbot.timer` plus `ahsanly-cert-renew.timer` reload logic.

## Nginx routing

- `ahsanli.com/` → Go API apex root decides geo-redirect or `/countries` fallback.
- `ahsanli.com/countries` → country selector.
- `eg.ahsanli.com/ar/` and `/en/` → Egypt storefront.
- `ahsanli.com/console` → staff console (not served on `eg.` subdomain).
- `eg.ahsanli.com/api/v1/countries/EG/...` → country-scoped Go API.
- `134.122.89.95/` → 301 to `https://eg.ahsanli.com/ar/`.
- `134.122.89.95/dashboard|admin|login|otp|...` → 301 to `https://ahsanli.com/console`.

Rate limits: catalog API `30r/m` with burst 60; login POST `5r/m` with burst 5.

## Telegram

The existing Egypt channel was kept with its old bot token (user decision). The channel title is **"أحسنلي | عروض مصر"**. The AHSANLY environment variable is `DEAL_TELEGRAM_TOKEN_AHSANLY_BOT`. One labelled test photo was sent as message `1831`.

## Operational limits

- Amazon cap: **120 requests/hour**.
- Blocked-source backoff: six hours, then one retry probe.
- Photo-only deal requirement: fresh direct observation + matching screenshot.
- No Windows project scripts are running.
- Egypt is the only enabled country.

## Useful checks

```bash
# Service health
systemctl is-active ahsanly-api.service ahsanly-web.service ahsanly-worker.service ahsanly-pipeline.service
systemctl show ahsanly-api.service ahsanly-web.service ahsanly-worker.service ahsanly-pipeline.service -p MemoryCurrent -p MemoryPeak -p NRestarts

# Database
runuser -u postgres -- psql -d ahsanly -tAc 'SELECT count(*) FROM core_listing'
runuser -u postgres -- psql -d ahsanly -tAc 'SELECT count(*) FROM core_publication'
runuser -u postgres -- psql -d ahsanly -c "SELECT code, enabled, health FROM core_retailer;"

# Logs
journalctl -u ahsanly-worker.service -n 50 --no-pager
journalctl -u ahsanly-pipeline.service -n 50 --no-pager

# Nginx
nginx -t
ss -ltn | grep -E ':80 |:443 '

# Recent fresh offers
runuser -u postgres -- psql -d ahsanly -c "SELECT count(*) FROM core_offerobservation WHERE observed_at >= now() - interval '1 hour' AND imported=false;"
```

## Cutover / rollback

See `../AGENTS.md` for the full cutover and rollback commands. The key rule: stop old worker/pipeline before the final `pg_dump`, restore into `ahsanly`, apply migrations, then start AHSANLY services and switch Nginx. Never restore an old dump over newer observations.
