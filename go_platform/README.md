# Go platform (incremental cutover)

**2026-11-04 live note:** 63,060 Amazon listings are currently Go-owned on the VPS; the legacy publisher is stopped. The retailer cap is 120 requests/hour while a six-hour backoff from the 14:29 UTC source block is active. This supersedes the smaller watchlist counts below. The HTTP fetcher verifies TLS and keeps its ALPN matched to HTTP/1.1 or HTTP/2; Chrome-like headers do not guarantee that Amazon will allow requests. See [server operations](../ops/SERVER.md) before changing the cap or deploying.

The optional [Surf comparison pilot](experiments/surfprobe/README.md) is isolated from the live worker. Its review and promotion criteria are in [Surf review](../ops/SURF_REVIEW.md).

The Go API is live on loopback `127.0.0.1:8083` and Nginx exposes only `/api/v1/`. It reads the existing PostgreSQL catalog. Product prices are marked fresh only for direct, in-stock observations within the country rule's freshness window. The curated country deals endpoints require a matching screenshot, explicit staff approval, and a currently eligible offer; imported, stale, rejected, and unreviewed candidates do not appear.

`cmd/worker` implements selected Amazon offer jobs on the existing PostgreSQL lease table. Its default writes direct observations and **shadow** deal candidates. `--live-candidates` requires both `DEAL_PUBLISH_ENABLED=1` and `DEAL_GO_PUBLISH_ENABLED=1`, plus the retailer's `publish_enabled` gate. It runs as `botafflite-go-worker.service` for a bounded 282-ASIN mobile/pilot watchlist: 16 hourly and 266 daily under a 30-request/hour source cap. Django's worker is stopped and disabled. Noon and Jumia are excluded until collection permissions and a reliable server source are verified. Do not run the Django worker concurrently on the same queue.

Every live Go listing also needs `extracted_attributes.go_owned=true`. The worker claim and observation transaction, candidate capture, and outbox preparation/send enforce this flag. `manage.py cutover_mobile_watchlist --enable-go` sets it only after checking legacy tracking is off for the selected ASINs. The isolated PostgreSQL tests assert that an unowned listing cannot produce a live candidate or publication. The old service continues for its other 62,771 tracked products.

The `internal/fetch` rate gate and cooldown design adapts relevant ideas from the user's Apache-2.0 `D:\scopeDpi` project. Its DPI scanner, proxy tunneling and browser wire-fingerprint modules are not product ingestion components. TLS/header imitation does not prove a retailer will allow the VPS IP or return a valid offer. A challenge or incomplete price and seller remains a failure.

`internal/sources/category.go` extracts Amazon category ASINs, titles, images, and a same-host next-page cursor. It does not treat search-result prices as verified offers. `cmd/discover` is a one-request, read-only pilot command. On 2026-11-03 the VPS parsed 24 identities and a next-page cursor from the selected Amazon Egypt mobile category. This is not an ongoing category crawl. The public site labels imported Kanbkam prices as reference readings and suppresses imported seller labels; unverified Noon/Jumia mappings have no outbound store button.

The public API chooses an affiliate link only from an enabled public channel in the retailer's country with an enabled route for that retailer. Otherwise it falls back to a verified merchant URL. Existing Noon/Jumia merchant URLs are **not** affiliate tracking links unless a real program link is supplied and verified.

`internal/publishing` contains an Arabic photo-caption formatter, streaming Telegram `sendPhoto` client, and a PostgreSQL publication outbox. The outbox prepares one publication per eligible route and observation only after explicit editorial approval, verifies the screenshot path and offer again before sending, records the Telegram message ID, retries an explicit rate limit, and marks an uncertain upload for manual review. `cmd/publish` remains a one-shot tool. `cmd/pipeline` is the live service loop: it captures one qualifying candidate, prepares one routed publication, sends one verified photo, and holds uncertain delivery for review. It requires `--apply`, `--allow-live-channels`, both environment gates and the Amazon retailer gate. The service is `botafflite-go-pipeline.service`; it opens Chromium only for a qualifying candidate. The user accepted keeping the existing token and waived the 48-hour OOM wait. `cmd/testphoto` previously sent a labelled test to Koupnak. Unit tests cover caption truncation, photo upload, rate limiting, uncertain responses, evidence paths, and price formatting; the race test passes.

`internal/capture` and `cmd/capture` provide a headless Chromium capture **only after** a candidate has an expected direct price and seller. The Go capture validates the Amazon URL, re-reads price and seller from the rendered page, rejects cookie/overlay obstruction and mismatches, crops to the product area, and marks the center price, buy-box price, seller, and limited-stock line. A one-product, read-only VPS test on 2026-11-03 produced an annotated screenshot with price EGP 460, seller Kidstown_egy, and one unit left. This diagnostic is not scheduled. The same capture library is also used by the explicitly invoked `cmd/testphoto` test (message 1830); no live Go deal publisher is scheduled. It uses the existing Playwright Chromium binary but does not invoke Python or Selenium. Chromedp v0.10.0 logged two nonfatal unknown navigation-event warnings against the installed browser; the captured image and data were inspected manually.

`cmd/testphoto` is an explicit one-shot test for an existing routed Telegram channel. It refreshes a boxed product-page screenshot against a direct in-stock observation no older than two hours, sends a caption that clearly says **test** with no discount claim, and records the attempt and Telegram message ID in `AuditEvent`. The public channel requires `--allow-public-channel`; sending requires `--send`. A second attempt for the same listing/channel within 24 hours is rejected, including uncertain delivery. The user approved the existing Koupnak channel for this test; message 1830 was sent on 2026-11-04. `release_gate --allow-public-photo-test` verifies the audit, PNG, message ID, and matching recent direct observation. This does not enable the deal publisher.

`cmd/worker --capture --once` claims one qualifying Amazon candidate, opens one headless browser under a PostgreSQL advisory lock, and rechecks the observation before recording the screenshot evidence. It requires both publication environment gates. Successful evidence capture enters `review`; `cmd/pipeline` processes the outbox only for staff-approved candidates. The independent `cmd/capture` remains the read-only diagnostic. A 2026-11-04 capture as service user `botdeals` under a 500 MB cap verified price EGP 460.00, seller `Kidstown_egy`, and one unit left.

`internal/admin` implements staff login with the existing Django password hash and confirmed TOTP device, replay protection, PostgreSQL sessions, Secure/HttpOnly cookies, Origin and CSRF checks. The Go admin API is live at `/api/v1/admin/`; Next `/console` shows operational data and supports audited watchlist, threshold and exact-match review changes. Django remains available for first-time TOTP enrollment and fallback admin. Nginx rate limits admin login POSTs. Go migration `0001_admin_session.sql` creates the session table. The API does not expose bot tokens or database credentials.

The console lists countries, Telegram destinations, and publication routes by country and optional retailer. Staff can create a **disabled** Telegram channel using a server environment-variable name, then create a disabled country-wide or retailer-specific route. Enable/pause changes use CSRF protection and audit records. Duplicate destinations/routes are rejected; cross-country routes are rejected. Channel activation requires an active country, valid Telegram destination, and a configured `DEAL_TELEGRAM_TOKEN_*` environment reference. The actual token never enters the dashboard or database. Amazon publication gates are on for the selected disjoint watchlist; Noon/Jumia gates remain off. Non-Telegram transports and country creation remain in Django until their Go flows are complete.

The Go worker also has a `--discover` mode for enabled Amazon category seeds. It uses database leases, page cursors, retailer request limits, and transactional listing upserts. New products start `paused` without offer jobs or deal posts. This path is **not scheduled** on the VPS; the continuous service runs offer checks only.

Schema migrations `core.0013_go_worker_leases` and `core.0014_publication_next_attempt` add capture and delivery leases plus a scheduled retry time. They were applied on the VPS after a local PostgreSQL backup on 2026-11-03. See [Go migration runbook](../ops/GO_MIGRATION.md).

## Build and test

```powershell
cd go_platform
go test ./...
$env:GOOS='linux'; $env:GOARCH='amd64'
go build -o out/botafflite-api-linux ./cmd/api
go build -o out/botafflite-worker-linux ./cmd/worker
go build -o out/botafflite-publish-linux ./cmd/publish
go build -o out/botafflite-discover-linux ./cmd/discover
go build -o out/botafflite-migrate-linux ./cmd/migrate
Remove-Item Env:GOOS,Env:GOARCH
```

The API reads `DATABASE_URL` or `DEAL_DB_*` from the protected server environment. Do not print these values. Unit: `go_platform/deploy/botafflite-go-api.service` (installed on the VPS).

## Current limits before replacing Django

- The API covers public catalog/search/detail/history and protected administration. Django still owns first-time TOTP enrollment, automated match suggestions, pilots, backups and summaries. Go offer checks and the verified photo pipeline run for the selected Amazon list.
- The Go worker, outbox, and admin flows passed targeted tests against an isolated PostgreSQL database. The user-approved public photo test passed; no simulated deal was posted. The user waived token rotation and the 48-hour OOM wait for this bounded cutover.
- Many imported listings have old prices and no product image. The public UI labels them as historical readings and does not claim they are offers.
- The legacy service still covers products outside the 282-ASIN Go watchlist. At 30 direct requests per hour, the ~63,000-product old catalog cannot be refreshed quickly on the current VPS/source connection; expand ownership in bounded cohorts with nonoverlapping legacy tracking.
- There is no claim that Go or a request-fingerprint library can bypass a source block. Monitor source success before expanding a watchlist.

## Rollback

For worker rollback, stop/disable `botafflite-go-worker.service`, then enable/start `botafflite-worker.service`. Do not run both. The previous Go API binary is `go_platform/bin/botafflite-api.before-match-review-20261103`; previous Next files are `web_next.before-match-review-20261103`; Nginx backup is `/etc/nginx/sites-available/botafflite.before-go-admin-20261103`. The legacy Amazon publisher is independent and remains active.
