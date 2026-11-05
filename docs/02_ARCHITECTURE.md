# 2. Current architecture and repository map

## Runtime overview

AHSANLY uses one Next.js instance, one Go API, one PostgreSQL database, and country-specific Telegram channels. All public countries are served by the same application stack; do not create a separate deployment per country.

| Component | Implementation | Production unit / bind |
| --- | --- | --- |
| Public web and console UI | Next.js 16, React, TypeScript | `ahsanly-web.service`, `127.0.0.1:3100` |
| Public/admin HTTP API | Go `net/http`, `pgx` | `ahsanly-api.service`, `127.0.0.1:8083` |
| Offer worker | Go worker with PostgreSQL jobs/leases | `ahsanly-worker.service` |
| Evidence and Telegram pipeline | Go worker capture + publishing outbox | `ahsanly-pipeline.service` |
| Data | PostgreSQL database `ahsanly` | Loopback only; application role `ahsanly_app` |
| Public routing/TLS | Nginx + Let's Encrypt | `ahsanli.com`, `eg.ahsanli.com`, `www.ahsanli.com`, legacy IP redirects |
| Country lookup | Local DB-IP Lite MMDB | `/etc/ahsanly/dbip-country-lite.mmdb` |
| Evidence | PNG files | `/var/lib/ahsanly/captures` |
| Backups | PostgreSQL custom-format dumps | `/var/backups/ahsanly/pg` |
| Source/builds | GitHub + Actions | `https://github.com/abdelrahman30x/Ahsanli`, `main` |

Next does not connect directly to PostgreSQL and does not need Telegram credentials. It calls Go over loopback using `API_INTERNAL_URL=http://127.0.0.1:8083`. Go reads `DATABASE_URL` or `DEAL_DB_*` from the protected environment.

## Request flow

1. Nginx terminates TLS and routes public country pages to Next.
2. Next server components call the country-scoped Go endpoints.
3. Go reads PostgreSQL and returns only currently eligible curated deals.
4. Product images are fetched through Next's allowlisted `/api/proxy-image` route, not arbitrary client-supplied hosts.
5. The apex root is handled by Go because it needs the Nginx-overwritten client IP and the saved country cookie.
6. `/console` is served only at the apex staff origin. Admin mutations go to Go with a staff cookie, exact origin, and CSRF token.

## Repository layout

- `go_platform/cmd/api` — API process.
- `go_platform/cmd/worker` — offer checks and optional one-shot diagnostic modes.
- `go_platform/cmd/pipeline` — continuous capture/outbox/photo-delivery loop.
- `go_platform/cmd/migrate` — embedded schema migration runner.
- `go_platform/cmd/capture`, `discover`, `probe`, `publish`, `testphoto`, `geoipverify` — bounded operational/diagnostic commands, not additional scheduled workers.
- `go_platform/internal/catalog` — public read models, country metadata, curated deals, cursor handling, geo lookup, DB connection.
- `go_platform/internal/httpapi` — public/admin HTTP route registration and public handlers.
- `go_platform/internal/worker` — job claiming, direct observation recording, candidate creation, discovery, evidence capture.
- `go_platform/internal/publishing` — editorially gated outbox, caption formatting, Telegram transport, delivery recovery.
- `go_platform/internal/admin` — staff sessions, CSRF/origin checks, operational edits, editorial review, authenticated evidence.
- `go_platform/internal/capture` — Chromium evidence capture and matching.
- `go_platform/internal/fetch` and `internal/sources` — retailer transport, parsing, rate/block handling.
- `go_platform/internal/schema/migrations` — ordered SQL migrations embedded into the migration binary.
- `web/app/[locale]` — localized public home, deals, deal detail, and about pages.
- `web/app/(global)/console` — staff UI at `/console`.
- `web/app/(global)/countries` — country selector.
- `web/components` — language switcher, mobile drawer, curated deal card.
- `web/lib/api.ts` — server-side Go client and shared display types/formatters.
- `web/lib/i18n.ts` — Arabic/English interface dictionary.
- `ops` — current server scripts/units plus retained historical files.
- `.github/workflows/build-release.yml` — tests and release publication.
- `docs` — canonical comprehensive documentation sections and generator.

## Production source layout

Current application symlinks:

- `/srv/ahsanly/go_platform` → `/srv/ahsanly/releases/<commit>/go_platform`
- `/srv/ahsanly/web_next` → `/srv/ahsanly/releases/<commit>/web_next`

The four service definitions continue to reference those stable symlink paths. `/var/lib/ahsanly/deploy/current-revision` records the active full commit. `/srv/ahsanly/pre-autodeploy` retains the initial pre-release-layout directories.

## Legacy material that must not be mistaken for production

The repository contains retained Python tests and old `botafflite-*` operational files for traceability. They are not the active production stack. In particular:

- Do not run the legacy Django worker or publisher concurrently with Go.
- Do not install old BotAfflite service units.
- Do not assume `tests/test_*.py` describes current Go/Next behavior.
- Do not use Koupnak, old IP bindings, or old `/root/botAfflite` paths in new features.
- Do not modify or delete the separate retained pre-Go workspace snapshot.

Historical documents may contain old caps, watchlist sizes, service states, or publication behavior. Current behavior and invariants in this documentation take precedence; live state must still be reconfirmed.
