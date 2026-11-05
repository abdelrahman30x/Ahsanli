# AHSANLY workspace guide

**Cutover completed:** 2026-11-05. AHSANLY | أحسنلي is the live public brand at `ahsanli.com`. Egypt is the first country at `eg.ahsanli.com/ar/` and `/en/`.

## Live state

| Component | Service / path | Bind |
| --- | --- | --- |
| Go catalog API | `ahsanly-api.service`, `/srv/ahsanly/go_platform/bin/ahsanly-api` | `127.0.0.1:8083` |
| Next.js public site | `ahsanly-web.service`, `/srv/ahsanly/web_next/server.js` | `127.0.0.1:3100` |
| Go price worker | `ahsanly-worker.service`, `/srv/ahsanly/go_platform/bin/ahsanly-worker --apply --live-candidates` | — |
| Go photo pipeline | `ahsanly-pipeline.service`, `/srv/ahsanly/go_platform/bin/ahsanly-pipeline --apply --allow-live-channels` | — |
| Node runtime | `/opt/ahsanly-node/bin/node` (symlink to `/opt/botafflite-node/bin/node`) | Local only |
| Chromium | `/opt/ahsanly-chromium/chrome-headless-shell` (symlink to `/opt/botafflite-chromium`) | Local only |
| PostgreSQL | database `ahsanly`, role `ahsanly_app` | `127.0.0.1:5432` |
| Secrets | `/etc/ahsanly/ahsanly.env` (mode `0600`, root-only) | — |
| Captures | `/var/lib/ahsanly/captures` | — |
| Backups | `/var/backups/ahsanly/pg` | — |
| DB-IP Lite | `/etc/ahsanly/dbip-country-lite.mmdb` | Updated monthly via `ahsanly-geoip-update.timer` |

DNS: `ahsanli.com`, `eg.ahsanli.com`, and `www.ahsanli.com` A records point to `134.122.89.95`. HTTPS certificate from Let's Encrypt covers all three domains.

Nginx serves the new site from `/etc/nginx/sites-available/ahsanly`. Old `134.122.89.95` IP-based requests redirect to `https://eg.ahsanli.com/ar/...`; old staff paths redirect to `https://ahsanli.com/console`. The previous BotAfflite site config is saved in `/etc/nginx/sites-available/botafflite` but not enabled.

## Telegram

The user chose to keep the existing Egypt Telegram channel and its old bot token, not create a new `@AhsanlyBot`. The channel title was renamed to **"أحسنلي | عروض مصر"**; its public handle remains the previous one. The new AHSANLY environment uses `DEAL_TELEGRAM_TOKEN_AHSANLY_BOT` in `/etc/ahsanly/ahsanly.env`, copied from the old protected env. One labelled test photo was sent as Telegram message `1831` after cutover.

## GitHub build and automatic deployment

Source: `https://github.com/abdelrahman30x/Ahsanli` (`main`). Every push runs `.github/workflows/build-release.yml`: Go tests, Linux AMD64 binaries, and the Next.js standalone build. A successful workflow publishes `deploy-<commit>` as the latest GitHub release with a bundle and SHA-256 manifest.

On the VPS, `ahsanly-deploy.timer` runs `/usr/local/sbin/ahsanly-deploy` every two minutes. It downloads only a new release, verifies its checksum, takes a PostgreSQL backup, applies embedded Go migrations, switches versioned symlinks under `/srv/ahsanly/releases/`, restarts the four application units, and rolls back the code symlinks if local health checks fail. It does not replace `/etc/ahsanly/ahsanly.env`, systemd units, Nginx, captures, or the database with an old snapshot.

```bash
systemctl status ahsanly-deploy.timer ahsanly-deploy.service
cat /var/lib/ahsanly/deploy/current-revision
journalctl -u ahsanly-deploy.service -n 80 --no-pager
# Pause automatic code deployment during maintenance:
systemctl disable --now ahsanly-deploy.timer
# Resume:
systemctl enable --now ahsanly-deploy.timer
```

Keep only one worker and one pipeline active. Deployment never enables the legacy BotAfflite units or changes the Amazon request cap.

## Curated public deals

The public site is a selected-deals storefront, not a general price-comparison catalog. A candidate enters `review` after a matching screenshot; staff must explicitly approve it in `/console` before it can appear publicly or enter the Telegram outbox. Rejected, unreviewed, imported, stale, wrong-country, or evidence-mismatched candidates never appear as current deals. Merchant-displayed discount and AHSANLY historical drop are separate facts. Existing sent Telegram history and publication keys remain unchanged.

## Operational limits (unchanged)

- Amazon source cap remains **120 requests/hour** with six-hour blocked-source backoff.
- Photo-only deal requirement remains: a Telegram post requires a fresh direct observation plus a matching boxed screenshot.
- No Windows BotAfflite Scheduled Task is re-enabled.
- Noon/Jumia remain paused/blocked.
- Egypt is the only enabled country.

## Quick checks

```powershell
# Public site
curl.exe -k --silent --show-error --output NUL --write-out '%{http_code}\n' https://eg.ahsanli.com/ar
curl.exe -k --silent --show-error --output NUL --write-out '%{http_code}\n' https://eg.ahsanli.com/en
curl.exe -k --silent --show-error --output NUL --write-out '%{http_code}\n' https://ahsanli.com/console

# Services
ssh -i C:\Users\AbdelrahmanMostafa\server_ssh root@134.122.89.95 'systemctl is-active ahsanly-api.service ahsanly-web.service ahsanly-worker.service ahsanly-pipeline.service'
ssh -i C:\Users\AbdelrahmanMostafa\server_ssh root@134.122.89.95 'systemctl is-active botafflite-go-api.service botafflite-go-worker.service botafflite-go-pipeline.service botafflite-next-preview.service || true'

# Database counts
ssh -i C:\Users\AbdelrahmanMostafa\server_ssh root@134.122.89.95 'runuser -u postgres -- psql -d ahsanly -tAc "SELECT count(*) FROM core_listing"'
ssh -i C:\Users\AbdelrahmanMostafa\server_ssh root@134.122.89.95 'runuser -u postgres -- psql -d ahsanly -tAc "SELECT count(*) FROM core_publication"'
```

## Cutover commands (record)

Final BotAfflite backup before cutover:
```bash
runuser -u postgres -- pg_dump -Fc -d botafflite -f /var/backups/ahsanly/pg/botafflite-final-cutover-$(date -u +%Y%m%d-%H%M%S).dump
```

Recreate `ahsanly` and restore:
```bash
systemctl stop ahsanly-api ahsanly-web ahsanly-worker ahsanly-pipeline
runuser -u postgres -- psql -c "DROP DATABASE IF EXISTS ahsanly;"
runuser -u postgres -- psql -c "CREATE DATABASE ahsanly OWNER ahsanly_app;"
runuser -u postgres -- pg_restore -d ahsanly -j 2 --no-owner /var/backups/ahsanly/pg/botafflite-final-cutover-YYYYMMDDsHHMMSS.dump
runuser -u postgres -- psql -d ahsanly -Atc "SELECT 'ALTER TABLE ' || quote_ident(schemaname) || '.' || quote_ident(tablename) || ' OWNER TO ahsanly_app;' FROM pg_tables WHERE schemaname='public';" | runuser -u postgres -- psql -d ahsanly
runuser -u postgres -- psql -d ahsanly -Atc "SELECT 'ALTER SEQUENCE ' || quote_ident(sequence_schema) || '.' || quote_ident(sequence_name) || ' OWNER TO ahsanly_app;' FROM information_schema.sequences WHERE sequence_schema='public';" | runuser -u postgres -- psql -d ahsanly
set -a; . /etc/ahsanly/ahsanly.env; set +a
/srv/ahsanly/go_platform/bin/ahsanly-migrate --apply
```

Start new stack and switch Nginx:
```bash
systemctl stop botafflite-go-worker botafflite-go-pipeline
systemctl enable --now ahsanly-api ahsanly-web ahsanly-worker ahsanly-pipeline
ln -sf /etc/nginx/sites-available/ahsanly /etc/nginx/sites-enabled/ahsanly
rm -f /etc/nginx/sites-enabled/botafflite
nginx -t && systemctl reload nginx
```

Send a labelled test photo:
```bash
set -a; . /etc/ahsanly/ahsanly.env; set +a
/srv/ahsanly/go_platform/bin/ahsanly-testphoto -listing=<ID> -channel=1 -browser=/opt/ahsanly-chromium/chrome-headless-shell -output=/var/lib/ahsanly/captures/test-$(date -u +%Y%m%d-%H%M%S).png -send -allow-public-channel
```

## Rollback

To revert public routing to the old BotAfflite Next site while keeping the AHSANLY database:
```bash
systemctl stop ahsanly-worker ahsanly-pipeline
ln -sf /etc/nginx/sites-available/botafflite /etc/nginx/sites-enabled/botafflite
rm -f /etc/nginx/sites-enabled/ahsanly
nginx -t && systemctl reload nginx
systemctl start botafflite-go-worker botafflite-go-pipeline
```

Do not restore the pre-cutover `botafflite` dump over `ahsanly` unless you intend to discard observations created after cutover.

## Retained snapshots

- Pre-Go BotAfflite workspace: `C:\Users\AbdelrahmanMostafa\Desktop\botAfflite-OLD-before-full-Go-2026-11-03` (do not modify).
- BotAfflite code and data remain on the VPS under `/root/botAfflite` but its Go/Next services are stopped and disabled.
