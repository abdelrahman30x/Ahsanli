# AHSANLY Complete Project and AI Handoff Guide

This standalone document describes the current AHSANLY product, architecture, data model, editorial rules, public interfaces, operations, testing, deployment, and safe extension process. It is generated from the numbered files in `docs/` and can be shared with another AI without any additional project files.

It contains no credentials, customer data, database dumps, or evidence images. Dated observations are not permanent health claims: inspect the current source and reconfirm live state before making operational changes.

---

## Contents

1. [1. Product identity and business rules](#1-product-identity-and-business-rules)
2. [2. Current architecture and repository map](#2-current-architecture-and-repository-map)
3. [3. Public frontend, console UI, and localization](#3-public-frontend-console-ui-and-localization)
4. [4. HTTP API reference](#4-http-api-reference)
5. [5. PostgreSQL data and domain model](#5-postgresql-data-and-domain-model)
6. [6. Offer evaluation, editorial review, and Telegram pipeline](#6-offer-evaluation-editorial-review-and-telegram-pipeline)
7. [7. Security and operational invariants](#7-security-and-operational-invariants)
8. [8. Development and testing guide](#8-development-and-testing-guide)
9. [9. GitHub deployment, server updates, and rollback](#9-github-deployment-server-updates-and-rollback)
10. [10. Safe extension and modification recipes](#10-safe-extension-and-modification-recipes)
11. [11. Current state, limitations, and discrepancies](#11-current-state-limitations-and-discrepancies)
12. [12. AI handoff and review checklist](#12-ai-handoff-and-review-checklist)

---

<!-- Source: docs/01_PRODUCT.md -->

# 1. Product identity and business rules

## What AHSANLY is

AHSANLY (Arabic: **أحسنلي**) is a multi-country, bilingual, curated-deals platform. Its tagline is **“أحسن عرض.. أحسن سعر”**. The public product is a small selection of worthwhile, verified store offers—not a large catalog, a general search engine, or a shopper-facing price-comparison tool.

Egypt is the first launched country. Its public storefronts are:

- `https://eg.ahsanli.com/ar/` — Arabic, right-to-left.
- `https://eg.ahsanli.com/en/` — English, left-to-right.
- `https://ahsanli.com/` — apex entry that uses a saved country preference or local IP geolocation.
- `https://ahsanli.com/countries` — permanent country selector.
- `https://ahsanli.com/console` — the only staff origin.

The purchased domain is spelled `ahsanli.com`; the brand spelling remains **AHSANLY** even though the hostname uses `ahsanli`.

## The customer promise

A visitor should be able to answer:

1. What is the product and its exact retailer listing?
2. What is its current item price and currency?
3. Which store and seller provide it?
4. When was it last checked successfully?
5. Is the percentage a discount displayed by the store or a historical drop measured by AHSANLY?
6. Why did AHSANLY select it?
7. How do I open the store page?

The platform may earn affiliate commission from eligible links. Shipping is excluded from the observed item price, and the customer is told to verify variant, seller, shipping, availability, and final price at the store.

## Editorial responsibility

Not every lower price is a deal worth publishing. Automated monitoring creates evidence and candidates; **AHSANLY staff make the publication decision**. Human approval is required for both website visibility and Telegram publication. A future ranking system may prioritize the review queue, but it must not silently become an automatic approval mechanism.

The platform does not claim that an item is the cheapest across stores. Historical comparison is an internal evidence and selection tool. Public copy focuses on the selected deal.

## Two different discount facts

Never merge or confuse these fields:

- **Store-displayed discount**: a percentage independently seen on the retailer page during evidence capture. Public label: “Store-displayed discount” / “خصم المتجر المعروض”.
- **AHSANLY historical drop**: the percentage difference between the current direct observation and the immediately previous valid direct in-stock observation for the same listing. Public label: “Verified price drop” / “انخفاض سعر موثق”.

If the store displayed no discount, do not invent one. If only a historical drop exists, do not describe it as a retailer promotion. A stored/imported price cannot be used as proof of a current deal.

## Public content rules

Only deals satisfying all current gates appear publicly:

- Correct enabled country and public-site gate.
- Enabled retailer with publication enabled.
- Go-owned listing.
- Fresh, direct, non-imported, in-stock observation.
- Current-offer pointer still references the winning observation.
- Matching screenshot path, price, and seller.
- Current rule version and enabled deal rule.
- Editorial state `approved` or previously queued/sent history that remains currently eligible.

Uncurated product IDs return 404 in the new public Next experience. An empty approved-deal list is valid and preferable to filler. The empty state promotes the country Telegram channel without fabricating discounts.

## Non-goals for the current product

- Shopper accounts, wishlists, saved searches, or personal alerts.
- Public cross-store comparison tables.
- Maximizing product count or claiming comprehensive price freshness.
- Publishing every qualifying percentage drop.
- Automatic editorial approval.
- Treating category/search-result prices as direct product evidence.
- Enabling Noon, Jumia, or another country merely because legacy rows exist.

## Brand and accessibility

The interface uses a warm off-white background, deep navy text, blue actions, amber commercial highlights, and green reserved for verified states. IBM Plex Sans Arabic and Manrope are self-hosted. The layout is mobile-first, keyboard navigable, and intended to meet WCAG AA contrast. Public pages remain primarily server-rendered; no large client-side UI framework is needed on the 1 GB VPS.

---

<!-- Source: docs/02_ARCHITECTURE.md -->

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

---

<!-- Source: docs/03_FRONTEND.md -->

# 3. Public frontend, console UI, and localization

## Public routes

| Route | Purpose |
| --- | --- |
| `/{locale}/` | Commercial hero, featured deal when selected, latest approved deals, store/category entry points, selection explanation |
| `/{locale}/deals` | Approved-deals archive with store/category chips and cursor pagination |
| `/{locale}/products/{listingID}` | Deal detail for an approved listing in the current country; otherwise 404 |
| `/{locale}/about` | Editorial and verification policy |
| `/countries` | Accessible country selector at the apex |
| `/console` | Staff operations and editorial review |
| `/console/login` | Staff login |

The old public catalog controls are intentionally absent: no prominent title/URL search, price range, verification dropdown, price sorting, product counts, or public price-history chart. The archive supports only lightweight store/category filters and an opaque `cursor` for the next page.

## Home-page composition

`web/app/[locale]/page.tsx` fetches country metadata and curated deals concurrently. It renders:

1. A navy commercial hero with a primary deals CTA and country Telegram link.
2. Three concise proof points: direct store reading, matching evidence, staff review.
3. A lead card for a deal explicitly marked `featured`, if present.
4. Latest selected deals, excluding the repeated featured card.
5. Enabled-store and curated-category links.
6. A three-step selection explanation.

If no deal exists, the page renders an honest empty state. It does not replace deals with general products.

## Deal cards and detail

`web/components/DealCard.tsx` displays image, store, retailer-supplied title, current item price, last check, verified status, and a single detail CTA. Merchant discount and historical drop use different labels and styling.

The deal detail page adds seller, exact store, editorial note, evidence facts, shipping exclusion, and the verified affiliate/store link. Public history charts are intentionally removed. Historical observations remain in PostgreSQL for internal evidence and debugging.

## Localization

`web/lib/i18n.ts` defines `ar` and `en`, direction, and all shared interface strings. Add new keys to the Arabic object first and mirror them in the typed English object. Do not translate retailer-supplied product titles.

The language switcher preserves the current path and query while replacing only the leading locale. The mobile drawer contains deals, stores, selection policy, about, Telegram, and country switching. Closed drawer content is removed from display to avoid off-screen overflow and keyboard focus.

## Metadata and sharing

Localized layouts emit canonical URLs and reciprocal `ar-EG`/`en` hreflang links. Deals and deal detail pages add their own canonical/alternate links; deal detail also emits Open Graph metadata. Only currently public approved deals should be indexed or included in future sitemap additions. Missing/expired deals return noindex/not-found behavior rather than presenting stale content as current.

## Styling and assets

`web/app/globals.css` contains the design tokens and reusable styles. Important tokens include `--bg`, `--ink`, `--action`, `--highlight`, `--verified`, spacing, radii, and the two font families. Green is reserved for verified offers. Amber is used for commercial emphasis, not as a verification substitute.

Fonts and `logo-mark.svg` live under `web/public`. Next's standalone packaging must include both `.next/static` and `public`; the CI workflow handles this explicitly.

## Image proxy

`/api/proxy-image?url=...` accepts only a fixed host allowlist for known retailer CDNs. It sets an upstream Referer and caches successful images. Do not turn it into an arbitrary URL proxy. When adding a retailer, add only its required HTTPS image hosts and test failure handling.

## Console experience

The console is intentionally operational rather than shopper-facing. Tabs cover overview, listings, rules, channels, exact-match reviews, deal candidates, and publications. The candidate review UI includes authenticated screenshot evidence, price/seller/store/time, separate discount facts, bilingual editorial notes, feature selection, approval, rejection, and post-approval editing.

All admin data is fetched client-side with same-origin cookies and no-store caching. Mutations send `X-CSRF-Token`; the server independently enforces session, host, origin, and CSRF.

## Frontend change checklist

When changing the public experience:

- Preserve curated-only data sources; do not reintroduce `listProducts` into public deal feeds.
- Keep Arabic and English copy, directions, metadata, and 360 px layouts in sync.
- Test empty, unavailable, missing-image, long-title, and no-store-link states.
- Keep pages server-rendered where practical and avoid new client-state dependencies.
- Run `npm run build` and browser checks in both locales before pushing.

---

<!-- Source: docs/04_API.md -->

# 4. HTTP API reference

All API paths are below `/api/v1`. Public responses are JSON with `X-Content-Type-Options: nosniff`. The API has a 12-second outer timeout; individual database operations use shorter contexts.

## Health and metadata

### `GET /api/v1/health`

Pings PostgreSQL. Returns `200 {"status":"ok"}` or `503 {"status":"unavailable"}`. Used by deployment health checks.

### `GET /api/v1/meta`

Returns enabled public countries and their metadata. Cache: public, 300 seconds.

### `GET /api/v1/countries/{code}/meta`

Returns country names, currency, timezone, default locale, hostname, retailers, categories, and optional Telegram URL. Unknown/unavailable country returns 404; database failure returns 503.

### `GET /api/v1/countries/{code}/categories`

Returns the same curated category list as `{"categories":[...]}`.

## Curated deals

### `GET /api/v1/countries/{code}/deals`

Parameters:

- `store` — optional retailer code, for example `amazon_eg`.
- `category` — optional curated category code.
- `featured=1` — optional featured-only filter.
- `limit` — 1–48; invalid/out-of-range values fall back to 24.
- `cursor` — opaque continuation returned as `next_cursor`.

The response is:

```json
{
  "items": [],
  "next_cursor": "opaque-when-more-results-exist",
  "has_more": false
}
```

Each item embeds product fields and deal fields:

- `id` — listing ID used in the public detail URL.
- `deal_id` — candidate/editorial ID.
- `title`, `product_id`, `store`, `store_name`, `country`, `country_name`.
- `price`, `currency`, `seller`, `observed_at`.
- `imported`, `fresh`, `drop_percent`, `image_url`.
- `url`, `affiliate_url`.
- `displayed_discount` — merchant-displayed percentage or null.
- `featured`, `editorial_note_ar`, `editorial_note_en`.
- `selected_at` — review/selection timestamp used for editorial ordering.

Ordering is `featured DESC`, then `selected_at DESC`, then unique `deal_id DESC`. The cursor contains that complete ordering position; clients must treat it as opaque. This prevents duplicate/omitted entries when adjacent deals share a timestamp.

The SQL query repeats all public eligibility gates. Unreviewed, rejected, imported, stale, wrong-country, non-current, evidence-mismatched, disabled, or non-Go-owned candidates are excluded.

### `GET /api/v1/countries/{code}/deals/{listingID}`

Returns `{"deal":{...}}` using the same eligibility rules. Wrong country or unapproved/ineligible listing returns 404. Invalid IDs return 400. Cache: public, 20 seconds.

## Compatibility catalog endpoints

These remain temporarily for compatibility and internal/legacy consumers:

- `GET /api/v1/products` and `/api/v1/products/{id}` default to Egypt.
- `GET /api/v1/countries/{code}/products` supports title/ID/URL search, store, category, price range, verification, sort, limit, and cursor.
- `GET /api/v1/countries/{code}/products/{id}` returns product detail and observation history.

The new Next public experience uses only curated deals endpoints. Do not use these compatibility endpoints to repopulate the public home with unreviewed products.

General cursor rules include the sort and unique listing ID. Invalid, oversized, mismatched, or malformed cursors return 400. Country codes are two uppercase letters; store/category codes use validated lowercase patterns.

## Country selection and apex routing

### `GET /` on the apex API host

- `www.ahsanli.com` permanently redirects to the apex.
- A valid saved `country_pref` cookie wins over IP detection.
- Otherwise Go looks up the Nginx-provided `X-Real-IP` in the local DB-IP database.
- Only an enabled matching country receives a temporary redirect.
- Unknown, unsupported, failed, or VPN/unknown IP cases go to `/countries`.
- Responses are private/no-store. Direct country/product links never geo-redirect.

### `GET /api/v1/choose-country?code=EG&locale=ar`

Validates an enabled country and sets a one-year `Secure`, `HttpOnly`, `SameSite=Lax` `country_pref` cookie for `.ahsanli.com`. It contains no authentication state. The response is a 303 redirect to the selected country/locale.

## Admin API

Admin endpoints are registered only when `DEAL_GO_ADMIN_ENABLED=1` and `DEAL_GO_ADMIN_ORIGIN` is an HTTPS origin.

- `POST/GET/DELETE /api/v1/admin/session` — login, current session, logout.
- `GET /api/v1/admin/overview`, `retailers`, `listings`, `rules`, `channels`, `countries`, `routes`, `candidates`, `matches`, `publications`.
- `PATCH /listings/{id}` — watch/pause/cadence.
- `PATCH /rules/{id}` — historical threshold; advances rule version.
- `POST/PATCH /channels` and `/routes` — create disabled destinations/routes and explicitly enable/pause them.
- `PATCH /candidates/{id}` — approve/reject a review candidate or edit approved/queued editorial metadata.
- `GET /candidates/{id}/evidence` — authenticated, confined screenshot download.
- `PATCH /matches/{id}` — exact-variant match decision.

Admin reads require a valid staff cookie. Mutations additionally require the exact configured HTTPS Origin/Host and a constant-time CSRF match. Admin JSON is always no-store. Evidence is private/no-store and constrained to the capture directory after symlink resolution.

## Error and caching conventions

- 400: invalid input/cursor.
- 401: staff login required.
- 403: origin/CSRF verification failed or invalid login.
- 404: missing, wrong-country, uncurated, or currently ineligible resource.
- 409: state conflict, duplicate destination/route, ineligible approval, or activation gate failure.
- 503: temporary database/service unavailability.

Public lists/detail cache for 20 seconds; metadata for 300 seconds; admin and geo decisions are not publicly cached.

---

<!-- Source: docs/05_DATABASE.md -->

# 5. PostgreSQL data and domain model

## General rules

The production database is `ahsanly`, with application role `ahsanly_app`. The active Go code preserves generic/historical table names (`core_*`, `auth_user`, `otp_*`) and existing IDs. Do not rename these tables as part of branding.

Go connects with `DATABASE_URL` or the `DEAL_DB_HOST`, `DEAL_DB_PORT`, `DEAL_DB_NAME`, `DEAL_DB_USER`, and `DEAL_DB_PASSWORD` environment variables. SSL is disabled only for the loopback/local socket connection. Never print or commit these values.

Most core tables predate the Go migration. `go_platform/internal/schema/migrations` contains only the incremental Go-owned changes, not a complete from-scratch legacy schema. An isolated integration database therefore needs the legacy base schema before applying the embedded migrations.

## Core relationships

```text
core_country
  ├─ core_retailer
  │    └─ core_listing
  │         ├─ core_offerobservation ─ core_currentoffer (current pointer)
  │         ├─ core_fetchjob ─ core_fetchattempt
  │         ├─ core_affiliatelink
  │         └─ core_dealcandidate ─ core_publication
  ├─ core_dealrule
  ├─ core_channel ─ core_publicationroute
  └─ core_categoryseed

core_category ─ core_listing
auth_user ─ go_admin_session / review and audit references
otp_totp_totpdevice ─ auth_user
```

## Country, store, and classification tables

### `core_country`

Key fields: `id`, unique two-letter `code`, `name_en`, `name_ar`, `currency`, `currency_label_ar`, `locale`, `enabled`, unique public `hostname`, IANA `timezone`, `default_locale`, and `enabled_public_site`.

A country can exist operationally without being publicly launched. Both enabled/public gates and a valid hostname matter when adding a storefront.

### `core_retailer`

Key fields: `code`, `domain`, `currency`, `enabled`, `permission_verified_at`, `max_requests_per_hour`, `health`, success/failure timestamps, `last_error_class`, `permission_reference`, `display_name_ar`, `country_id`, `offer_source`, and `publish_enabled`.

`enabled` controls source operation; `publish_enabled` is a separate publication gate. `offer_source=server_http` allows the VPS worker. Other values such as dormant Windows HTTP configuration must not be treated as active monitoring.

### `core_category`

Small curated taxonomy: `code`, `name_ar`, `name_en`, optional `parent_code`, `position`. Current seeded categories are phones, electronics, home, and fashion. A listing's `category_id` is optional; uncertain products remain unclassified.

### `core_categoryseed`

Amazon category discovery configuration: collection URL, pagination cursor, priority, enabled state, last success, and retailer. Discovery is implemented but not continuously scheduled.

## Listing and price evidence

### `core_listing`

Important fields: `retailer_id`, `retailer_product_id`, `original_url`, retailer-supplied `title`, JSONB `extracted_attributes`, `tracking_state`, `cadence`, `last_checked_at`, `canonical_product_id`, and `category_id`.

`extracted_attributes` can contain `image_url` and the critical `go_owned=true` ownership flag. Live candidate capture and publication require this flag. `tracking_state=watch` is required for worker claims; new discovered listings start paused.

### `core_offerobservation`

Append-only price evidence: `listing_id`, numeric `price`, `currency`, `seller`, `stock_status`, optional source-displayed `displayed_discount`, `source`, `observed_at`, `imported`, and legacy reference fields.

The current deal evaluation compares the new observation only with the immediately previous directly verified in-stock observation for the same listing. Imported/Kanbkam hints and other stores cannot establish a historical deal.

### `core_currentoffer`

A listing-to-current-observation pointer (`listing_id`, `observation_id`, `updated_at`). Public deals and publication require the winning observation to remain this current pointer. This prevents an older candidate from being sold as the latest offer.

### `core_dealrule`

Country rule fields include `historical_percent`, `freshness_hours`, `version`, and `enabled`. Legacy cash-floor/cross-store/cooldown columns still exist, but the active per-listing deal decision uses the direct previous price, historical threshold, freshness window, and current gates. Changing the threshold advances `version`; older candidates fail current-version checks.

### `core_dailypricesummary`

SQL-generated daily min/max/sample-count rollups. These are summaries, not proof of a fresh offer.

## Jobs and source health

### `core_fetchjob`

`kind` is `offer` or `discover`; fields include `status`, `priority`, `due_at`, `lease_until`, `retry_count`, `last_result`, `last_error_class`, `listing_id`, and `seed_id`. Claims use `FOR UPDATE ... SKIP LOCKED` and a lease. This supports safe work claiming without a second worker.

### `core_fetchattempt`

Records start/finish time, result, error class, HTTP status, bytes, elapsed time, and metadata. The hourly count of attempts enforces the retailer's stored request ceiling. Source `health=blocked` suppresses normal claims until the six-hour backoff expires.

## Candidate and editorial data

### `core_dealcandidate`

Key groups:

- Linkage: `winning_observation_id`, `canonical_product_id`.
- Evaluation: `compared_observations`, `historical_baseline`, `rule_version`, `reason`, `percent`, `state`.
- Evidence: `evidence_path`, `evidence_price`, `evidence_seller`, `evidence_discount`, `limited_stock`.
- Capture reliability: `capture_retries`, `capture_lease_until`, `created_at`.
- Editorial: `reviewer_id`, `reviewed_at`, `featured`, `editorial_note_ar`, `editorial_note_en`.

Migration `0005_editorial_deal_review.sql` adds editorial fields and moves unissued captured/queued work to review while preserving candidates with sent publication history. Indexes support the editorial queue and featured/approved public feed.

## Telegram and publication tables

### `core_channel`

Fields include name, country, platform, numeric `telegram_chat_id`, `destination_id`, `bot_token_env`, `enabled`, `private_test`, validated `public_url`, and `message_locale`.

Only the environment-variable **name** is stored, never the bot token. Publishing should use the numeric chat ID so a public-handle change does not break delivery. Private test channels and public live channels are distinguished explicitly.

### `core_publicationroute`

Connects a country and channel, optionally restricted to one retailer, with an independent `enabled` flag. New routes start disabled. Cross-country routes are rejected.

### `core_affiliatelink`

Maps listing + channel to a `tracking_url` and `verified_at`. Public link selection and Telegram outbox require verified links and enabled public routes. The site may fall back to a valid direct merchant URL where the existing query permits it.

### `core_publication`

Fields include `candidate_id`, `channel_id`, `canonical_product_id`, unique `publication_key`, `status`, `screenshot_path`, attempts, errors, Telegram/provider message IDs, delivery lease, retry time, creation, and sent time.

The key `offer:<channelID>:<observationID>` prevents duplicate posts for the same observation/channel. Never regenerate historical keys during a rebrand or code rollback.

## Staff, audit, and exact matching

- `auth_user` supplies Django-compatible staff credentials and active/staff flags.
- `otp_totp_totpdevice` stores confirmed TOTP configuration and replay state.
- `go_admin_session` stores only a SHA-256 session-token hash, CSRF token, user, creation, and expiry.
- `core_auditevent` stores actor, action, subject, JSON details, and time for administrative decisions.
- `core_canonicalproduct` and `core_matchdecision` support exact brand/model/variant matching. Cross-store comparison is not part of the public deal calculation.
- `core_legacyalert` preserves old alert references without turning them into Go publications.

## Retained Kanbkam tables

`core_kanbkamcategory`, `core_kanbkamproduct`, `core_kanbkamproductcategory`, and `core_kanbkampricechange` are imported/historical hint data. They are not direct offer evidence and must never independently qualify a public or Telegram deal.

## Migration discipline

Migrations are embedded with `go:embed`, sorted by filename, and applied transactionally under a PostgreSQL advisory lock. `go_schema_migrations` records names. Use a new ordered file for every change; never edit an already deployed migration. Prefer additive, backward-compatible changes so code rollback against the same newer database remains safe.

---

<!-- Source: docs/06_DEAL_PIPELINE.md -->

# 6. Offer evaluation, editorial review, and Telegram pipeline

## Process separation

Two long-running units have different responsibilities:

- `ahsanly-worker.service` runs `ahsanly-worker --apply --live-candidates` for offer checks and candidate creation. It does not need to open Chromium for routine price checks.
- `ahsanly-pipeline.service` runs `ahsanly-pipeline --apply --allow-live-channels` and performs evidence capture, publication preparation, and one photo send per loop.

Only one of each may run. Do not start legacy Python/Django publishers or another Go worker against the same queue.

## Offer job lifecycle

1. The worker selects a due/expired-lease Amazon Egypt offer job using a PostgreSQL transaction and `SKIP LOCKED`.
2. Eligibility requires a watched listing, enabled server-HTTP retailer, allowed source health, and available hourly attempt budget.
3. Live candidate mode additionally requires retailer publication and `go_owned=true`.
4. The job becomes `leased` for three minutes while one source request is attempted.
5. A valid direct result writes an observation and current-offer pointer transactionally, records the attempt, and schedules the next cadence.
6. Source failure records error/backoff information. Incomplete price/seller, verification/challenge pages, or invalid variants are not successful deals.

The first valid direct price is a baseline. For a later direct in-stock observation, the worker compares it to the immediately previous valid direct in-stock observation for that same listing. Exact integer cents/basis-point arithmetic prevents floating-point threshold errors.

## Candidate state machine

Typical live path:

```text
new
  → capturing
  → review             (matching screenshot and evidence saved)
  → approved           (explicit staff decision)
  → queued             (outbox rows prepared for eligible channels)
  → core_publication: pending → sending → sent
```

Alternative states:

- `shadow` — non-live candidate used for internal evaluation; never public/published.
- `capture_retry` — capture can be retried, up to three attempts.
- `held` — invalid, stale, failed, missing evidence/route, or otherwise needs attention.
- `rejected` — staff rejected the candidate; it cannot enter the outbox.

A candidate does not become approved merely because its percentage exceeds the threshold. Approval is an editorial responsibility.

## Evidence capture

Capture uses one browser at a time under a PostgreSQL advisory lock, respecting the VPS memory limit. It:

1. Reclaims only eligible `new`/retry/expired-capture candidates.
2. Rechecks current direct price, seller, stock, freshness, rule version, retailer, country, and Go ownership.
3. Opens the exact Amazon listing in headless Chromium.
4. Re-reads rendered price and seller and rejects mismatch, overlay obstruction, or invalid evidence.
5. Writes a cropped/boxed PNG and stores path, matching price/seller, optional store-displayed discount, and limited-stock information.
6. Rechecks the observation before committing `review`.

Evidence paths are confined to `/var/lib/ahsanly/captures`. The public API does not expose arbitrary filesystem paths; the console's evidence endpoint requires staff authentication and checks resolved paths stay under the capture root.

## Editorial approval

The staff endpoint accepts `approved` or `rejected` only from `review`. Approval rechecks the live candidate conditions and rejects stale/ineligible work with 409. It records reviewer, review time, featured flag, and optional bilingual notes in an auditable transaction.

Approved or queued deals can receive editorial-note/featured edits without repeating the state transition. Replaying an approval is rejected. Rejection clears featured status. Existing pending publication rows cannot bypass the candidate-state checks.

## Outbox preparation

`Outbox.PrepareOne` selects one `approved` candidate and repeats:

- Historical baseline and threshold validity.
- Current-offer identity, direct/non-imported/in-stock/fresh observation.
- Evidence path/price/seller match and valid PNG file constraints.
- Retailer, country, Go ownership, rule, and publication gates.
- Enabled Telegram channel, country/retailer route, verified affiliate link.

It creates at most one publication per eligible channel using the stable `offer:<channel>:<observation>` key and moves the candidate to `queued`. Missing evidence/routes result in a held state rather than a text-only post.

## Photo send and delivery safety

Before sending, `claimPending` repeats the same offer, evidence, route, and state checks. It requires the configured token environment variable and numeric/effective destination, validates the evidence file, and leases the publication as `sending`.

The caption includes store, retailer title, current item price/currency, seller, product URL, optional historical drop, optional store-displayed discount, and limited stock. It does not claim cheapest-market status. Photo-only policy remains mandatory.

- A confirmed Telegram response stores the message ID and `sent` status.
- Explicit Telegram rate limits can receive a scheduled retry, bounded by attempt policy.
- A transport timeout or ambiguous result becomes `uncertain`, never an automatic duplicate upload.
- Expired sending leases are recovered as `uncertain` for manual review.
- Missing/revoked offer evidence or routes cause `held`.

## Public visibility versus Telegram history

Approved and currently eligible queued deals can appear publicly. Once the observation becomes stale, non-current, out of stock, or evidence/gates fail, the deal disappears from public API/detail responses. Already sent Telegram messages remain historical records; the website cannot retract or rewrite Telegram history.

## Safe testing

Never manufacture a production candidate or send a live photo just to prove the pipeline. Use unit tests, an isolated PostgreSQL database, and local screenshot/browser fixtures. A live labelled test photo is a separate explicit operational action requiring approval, not a routine deployment check.

---

<!-- Source: docs/07_SECURITY_OPERATIONS.md -->

# 7. Security and operational invariants

## Secrets and files

Production secrets are in `/etc/ahsanly/ahsanly.env`, root-only mode `0600`. This includes the database password, application secret, and Telegram tokens. Never print, copy into chat, commit, or put them in documentation. The repository contains only `ops/ahsanly.env.example` placeholders.

Environment files matching `.env`, `*.env`, and `*.env.*` are excluded by Git. Databases, build outputs, captures, logs, and local keys are also excluded. If a real environment file is accidentally committed, removing only the latest file is insufficient: remove it from history and assess/rotate exposed credentials.

Do not copy local source over newer server source without comparison. Routine deployment now uses reviewed GitHub releases, not ad-hoc whole-workspace uploads.

## Staff authentication

Admin access requires an active staff `auth_user`, Django-compatible password hash verification, and a confirmed TOTP device. TOTP replay state is updated transactionally. Sessions last eight hours and store only a token hash in PostgreSQL.

The `__Host-ahsanly_admin` cookie is `Secure`, `HttpOnly`, `SameSite=Strict`, and path-rooted. Every mutation requires:

- A valid active staff session.
- Exact configured HTTPS Origin and Host.
- A valid `X-CSRF-Token` compared in constant time.

Nginx rate-limits admin login POSTs. Country subdomains do not serve staff endpoints; use `https://ahsanli.com/console`.

## Editorial and retailer gates

A valid deal requires all applicable gates, not just one:

- Country enabled and publicly enabled.
- Retailer enabled and publication enabled.
- Listing watched/Go-owned where required.
- Current direct in-stock fresh observation.
- Enabled current-version rule.
- Matching evidence.
- Explicit staff approval.
- Enabled channel and route, verified affiliate link for publication.
- Global `DEAL_PUBLISH_ENABLED=1` and `DEAL_GO_PUBLISH_ENABLED=1` for live pipeline operation.

Adding any one flag must not bypass the others. Administrative enable operations are audited and new channels/routes begin disabled.

## Source and infrastructure limits

The source worker enforces the retailer's stored hourly request ceiling using recorded attempts and applies a six-hour blocked-source backoff before another probe. Do not increase caps, weaken challenge detection, or add concurrent workers as part of UI/deployment changes.

The VPS has limited memory. Go API/worker and Next have systemd memory limits; Chromium pipeline has a separate 500 MB cap and one-browser advisory lock. Do not run production builds on the VPS; GitHub Actions builds the artifacts.

No project scripts or scheduled tasks should be running on the Windows PC. Do not re-enable BotAfflite Windows tasks during development or deployment.

## Data and publication integrity

- Preserve listing, observation, candidate, publication, channel, and audit IDs.
- Preserve Telegram message IDs and publication deduplication keys.
- Never restore an old database snapshot over observations created after cutover.
- Code rollback uses the same current database; it must not roll back data.
- Keep migrations additive/backward-compatible with the previous release where possible.
- Never turn failed/missing screenshot evidence into a text-only deal.
- Imported/Kanbkam data is a hint, not a fresh direct observation.

## Web/network protections

- Public API uses validation, bounded limits, timeouts, and Nginx rate limits.
- Opaque cursors are validated for sort, ID, and shape.
- Image proxy uses an explicit retailer-CDN host allowlist.
- Admin evidence checks symlinks and path confinement, uses no-store, and requires a session.
- Geo lookup trusts only Nginx's overwritten client-IP header; direct product links never geo-redirect.
- Country preference cookie contains no authentication state.
- HTTPS covers apex, Egypt, and www; old IP public/staff paths redirect to the appropriate branded origin.

## Operational review before changes

Reconfirm current service state, source health, queue, publications, backups, and memory. A historical statement such as “Amazon is healthy” or “the cap is X” is not sufficient. Use `systemctl`, `journalctl`, and read-only SQL metadata/status queries without displaying secrets.

Do not fix unrelated operational discrepancies silently during a feature task. Report them and obtain a scoped decision.

---

<!-- Source: docs/08_DEVELOPMENT_TESTING.md -->

# 8. Development and testing guide

## Prerequisites

- Go version matching `go_platform/go.mod` (the module currently declares Go 1.27).
- Node.js 24 and npm using `web/package-lock.json`.
- Python 3 for the documentation generator; no third-party Python package is needed.
- PostgreSQL only for isolated integration tests, not for the routine Next build.
- Chromium only for explicit capture diagnostics; it is not needed for unit tests or Next development.

## Local setup

```bash
# Go
cd go_platform
go test ./...

# Web
cd ../web
npm ci
npm run build

# Documentation, from repository root
cd ..
python docs/build_docs.py
python docs/build_docs.py --check
```

`next.config.ts` uses `API_INTERNAL_URL` or `http://127.0.0.1:8083`. For local UI development against a remote read-only API, set `API_INTERNAL_URL` explicitly and understand that remote curated data may be empty. Do not expose admin credentials to Next.

## Go tests

`go test ./...` runs unit tests and skips PostgreSQL integration tests unless `DEAL_GO_INTEGRATION=1`. Unit coverage includes caption formatting, Telegram responses, evidence path safety, source parsing/fetch behavior, admin password/TOTP/origin helpers, migration logic, and exact threshold arithmetic.

Relevant integration tests cover:

- Staff session, CSRF, TOTP replay, admin edits, and audit records.
- Editorial approval eligibility and review persistence.
- Unowned/unreviewed candidate publication prevention.
- Approved-deal visibility, merchant-discount field, wrong-country isolation, stale withdrawal, and detail 404 behavior.
- Cursor pagination without duplicate listing IDs.
- Private/live channel routing and publication deduplication.
- Uncertain delivery recovery without Telegram HTTP.
- Direct observation baselines, shadow/live candidates, and ownership gates.

## Isolated PostgreSQL requirement

Integration tests intentionally retain the historical isolated database name `botafflite_go_test`. They refuse to run if `current_database()` does not match that name. Never point them at production `ahsanly`.

The isolated database needs the legacy base schema plus embedded migrations. Tests truncate their own fixture tables. Use a local/test role or a local PostgreSQL socket; do not place production credentials in shell history or documentation.

Typical environment shape (use only test values):

```bash
export DATABASE_URL='postgres://test_user:test_password@127.0.0.1:5432/botafflite_go_test?sslmode=disable'
export DEAL_GO_INTEGRATION=1
cd go_platform
go test ./internal/admin ./internal/publishing ./internal/worker
```

The current GitHub release workflow runs the ordinary Go suite, not the stateful isolated PostgreSQL suite. Run the isolated suite for schema, editorial, API-query, or publication changes before pushing. Never use production data or send Telegram during these tests.

## Frontend verification

`npm run build` performs compilation, TypeScript checks, and static/dynamic route generation. For UI changes, also exercise real browser behavior:

- 360 px mobile and desktop widths.
- Arabic RTL and English LTR.
- Mobile drawer open/close, keyboard focus, and no horizontal overflow.
- Empty deals, API unavailable, long title, missing image, and missing link states.
- Store/category chips and cursor continuation.
- Correct store-discount versus historical-drop wording.
- Deal detail and uncurated 404.
- Canonical and reciprocal hreflang metadata.
- Console login and, with authorized staff access, the editorial review queue.

No real Telegram post is required for a frontend release.

## Documentation verification

Edit the numbered `docs/*.md` sources, then regenerate the standalone file:

```bash
python docs/build_docs.py
python docs/build_docs.py --check
```

The builder is deterministic and can run from any current directory. CI uses `--check`; a source-section edit without regeneration fails the release. To make a separate portable copy:

```bash
python docs/build_docs.py --output /path/to/AHSANLY_AI_GUIDE.md
```

## Legacy tests

The root `tests/` directory contains retained Python tests for historical BotAfflite/research components whose source is not part of the active AHSANLY runtime. Do not treat a blanket `pytest` run as the active release test command. Add current Go tests beside the affected Go package and use the existing Next build/browser process for web changes.

## Pre-push checklist

1. Inspect the diff and update comments/docs describing changed behavior.
2. Run `git diff --check`.
3. Run Go unit tests and relevant isolated integration tests.
4. Run Next production build and browser scenarios for UI changes.
5. Regenerate/check documentation when architecture, commands, APIs, rules, or operations change.
6. Push `main`; GitHub builds and publishes the release.
7. Confirm the VPS deploys the full commit and all four units remain healthy.

---

<!-- Source: docs/09_DEPLOYMENT_ROLLBACK.md -->

# 9. GitHub deployment, server updates, and rollback

## Release pipeline

Routine production deployment starts with a push to `main` in `abdelrahman30x/Ahsanli`. `.github/workflows/build-release.yml` uses a production concurrency group and performs:

1. Repository checkout.
2. Documentation freshness check (`python docs/build_docs.py --check`).
3. Go setup from `go.mod` and `go test ./...`.
4. Linux AMD64, CGO-disabled, trimmed builds for `api`, `migrate`, `worker`, and `pipeline`.
5. Node 24 setup, `npm ci`, and `npm run build`.
6. Next standalone packaging, including `.next/static` and `public`.
7. One `ahsanly-linux-amd64.tar.gz` bundle plus SHA-256 `manifest.json` containing full commit and build time.
8. A `deploy-<full-commit>` GitHub release marked latest.

A failed test/build prevents release publication. Release assets do not contain environment files, database dumps, Nginx configuration, captures, or Telegram credentials.

## VPS automatic installer

`ahsanly-deploy.timer` runs `ahsanly-deploy.service` every two minutes with a small randomized delay. The service executes `/usr/local/sbin/ahsanly-deploy`, installed from `ops/ahsanly-deploy.sh`.

For each new latest release it:

1. Uses a nonblocking `flock` to prevent overlapping installers.
2. Downloads release metadata, manifest, and bundle over HTTPS.
3. Validates full commit shape and SHA-256 before extraction.
4. Rejects unsafe archive paths and verifies all application binaries and `web_next/server.js`.
5. Runs `ops/ahsanly-backup.sh`, which writes and validates a PostgreSQL custom-format backup.
6. Loads the protected environment without printing it and applies embedded migrations.
7. Moves the payload to `/srv/ahsanly/releases/<commit>`.
8. Atomically switches the stable Go and Next symlinks.
9. Restarts exactly `ahsanly-api`, `ahsanly-web`, `ahsanly-worker`, and `ahsanly-pipeline`.
10. Checks all units, local `/api/v1/health`, and local `/ar`.
11. On failure, restores the previous code symlinks and restarts the previous code without restoring old data.
12. Writes `/var/lib/ahsanly/deploy/current-revision` only after successful health checks.

If the latest manifest already equals the active revision, the installer exits without backup/restart.

## Normal operator commands

```bash
# Active source revision and timer
cat /var/lib/ahsanly/deploy/current-revision
systemctl list-timers ahsanly-deploy.timer

# Logs and immediate poll
journalctl -u ahsanly-deploy.service -n 100 --no-pager
systemctl start ahsanly-deploy.service

# Pause/resume automatic deployment during maintenance
systemctl disable --now ahsanly-deploy.timer
systemctl enable --now ahsanly-deploy.timer

# Application health
systemctl is-active ahsanly-api ahsanly-web ahsanly-worker ahsanly-pipeline
systemctl show ahsanly-api ahsanly-web ahsanly-worker ahsanly-pipeline \
  -p MemoryCurrent -p MemoryPeak -p NRestarts
```

## Manual code rollback

Disable the timer first if the newer latest release should not automatically return. Select an existing known-good full commit under `/srv/ahsanly/releases`:

```bash
revision=<full-commit>
test -x /srv/ahsanly/releases/$revision/go_platform/bin/ahsanly-api
test -f /srv/ahsanly/releases/$revision/web_next/server.js

ln -sfn /srv/ahsanly/releases/$revision/go_platform /srv/ahsanly/go_platform.next
mv -Tf /srv/ahsanly/go_platform.next /srv/ahsanly/go_platform
ln -sfn /srv/ahsanly/releases/$revision/web_next /srv/ahsanly/web_next.next
mv -Tf /srv/ahsanly/web_next.next /srv/ahsanly/web_next

systemctl restart ahsanly-api ahsanly-web ahsanly-worker ahsanly-pipeline
printf '%s\n' "$revision" > /var/lib/ahsanly/deploy/current-revision
```

Run local health checks afterward. Code rollback does not reverse schema migrations and must not discard newer observations. This is why migrations should remain compatible with the previous release.

## Database backups

`ops/ahsanly-backup.sh` uses `pg_dump` custom format, validates the archive with `pg_restore --list`, writes SHA-256, and prunes old dumps after the retention period. Daily/restore-test timers also exist. Backups currently have no assumed off-server destination; do not claim off-site disaster recovery unless separately configured and verified.

## Historical cutover

The BotAfflite-to-AHSANLY database cutover was a one-time operation: stop writers, take a final consistent snapshot, restore into `ahsanly`, preserve IDs, apply migrations, then start only the new units. Those commands are retained in `AGENTS.md` and `ops/SERVER.md` for history. They are not the routine deployment process.

Never restore the pre-cutover `botafflite` dump over `ahsanly` unless intentionally discarding all post-cutover data; that is not a valid ordinary rollback.

---

<!-- Source: docs/10_EXTENSION_GUIDE.md -->

# 10. Safe extension and modification recipes

This section is intended for a future developer or AI proposing changes. Read current code and reconfirm live state before editing. Keep changes scoped and preserve product/security invariants.

## Change public branding, copy, or layout

1. Edit shared strings in `web/lib/i18n.ts`; mirror every key in Arabic and English.
2. Edit the relevant server component under `web/app/[locale]` or reusable component under `web/components`.
3. Use tokens/components in `globals.css`; avoid one-off hardcoded colors that violate verified/action semantics.
4. Keep product titles unchanged and do not introduce public comparison/catalog feeds.
5. Test 360 px, desktop, RTL/LTR, keyboard drawer, empty states, canonical/hreflang, and no horizontal overflow.
6. Run `npm run build`, update documentation, regenerate the guide, then push.

## Add a field to curated deals API

1. Identify the authoritative database column; do not derive one discount fact from the other.
2. Extend the `Deal` struct in `go_platform/internal/catalog/deals.go`.
3. Extend the `SELECT` field list and `scanDeal` in exactly the same order.
4. Preserve cursor ordering and eligibility conditions.
5. Add/update the TypeScript `Deal` type in `web/lib/api.ts` only if the UI consumes it.
6. Add isolated integration coverage proving field value, country isolation, and no stale leakage.
7. Document the response field in `docs/04_API.md` and regenerate the handoff.

## Add or change a database migration

1. Create a new ordered file in `go_platform/internal/schema/migrations`; never modify an applied file.
2. Prefer additive columns/tables/indexes and safe defaults. Preserve IDs and historical publication keys.
3. Use `IF NOT EXISTS` where appropriate and consider behavior while old code is still running during deployment.
4. Test against an isolated database with the legacy base schema. Test both migration and old/new code compatibility where rollback matters.
5. Do not include a data backfill that manufactures deal eligibility or auto-approves candidates.
6. Document lifecycle/schema changes and ensure the automatic deployment backup succeeds.

## Add a country

1. Insert/configure `core_country` with a unique code, hostname, IANA timezone, default locale, currency, and public-site state.
2. Configure DNS/TLS and Nginx before enabling the hostname publicly.
3. Add that country's retailers and curated categories only with verified source data.
4. Create a disabled Telegram channel and disabled route; validate public URL and message locale.
5. Verify country-scoped API isolation, wrong-country 404, selector/cookie behavior, hreflang/canonicals, and empty states.
6. Do not copy Egypt observations or deals into the new country.
7. Enable public visibility only after explicit launch review.

## Add a retailer

1. Confirm collection permission and a reliable source from the VPS. A merchant URL or imported hint is not proof.
2. Create the retailer disabled, with country, domain, currency, source, request ceiling, and publication gate off.
3. Implement parser/direct observation support with exact variant identity, price, seller, stock, source time, and challenge/error handling.
4. Keep new listings paused until explicitly watched. Establish direct baselines before evaluating drops.
5. Add only required image CDN hosts to the proxy allowlist.
6. Test blocked/stale/incomplete responses, hourly rate enforcement, and no cross-country publication.
7. Enable source and publication separately after verified examples and operational approval.

## Change deal selection thresholds

Use the console rule edit or the corresponding audited admin endpoint. This advances `core_dealrule.version`; existing candidates with older versions stop passing current checks. Do not reduce evidence/freshness requirements to increase deal volume. Test exact-cent threshold boundaries and verify that editorial approval remains mandatory.

## Add or change a Telegram channel

1. Store the token only in the protected server environment; put only its environment-variable name in `core_channel`.
2. Prefer the numeric chat ID for delivery so handle changes do not break publication.
3. Create channels/routes disabled, verify country/retailer scope, public URL, locale, and bot administrator access.
4. Reconcile any uncertain deliveries before switching writers.
5. Keep one publisher and preserve old publication keys/message IDs.
6. Do not send an unlabelled test or manufacture a discount.

## Change Telegram captions

Edit `internal/publishing/caption.go`, preserving price, currency, seller, verified product URL, store, and evidence-based fields within Telegram's photo-caption limit. Keep store-displayed discount and historical drop in separate lines. Run caption tests, including long-title truncation and invalid URL rejection. Never replace a failed photo with a text-only deal.

## Change deployment logic

1. Edit `ops/ahsanly-deploy.sh` and its systemd unit/timer sources.
2. Preserve checksum verification, path safety, locking, backup, migration, single-writer restart, and health rollback.
3. Test archive validation and failure paths on a nonproduction target when possible.
4. Install the updated bootstrap script/unit deliberately; application releases do not self-update arbitrary server configuration.
5. Document operator commands and rollback changes.

## Prohibited shortcuts

- Do not enable legacy workers, Windows tasks, or another publisher to make tests easier.
- Do not bypass source blocks or raise request limits as a side effect.
- Do not auto-approve old candidates to populate the homepage.
- Do not treat build success alone as production verification.
- Do not restore an old DB over newer observations.
- Do not print secrets while gathering diagnostics.

---

<!-- Source: docs/11_CURRENT_STATE_LIMITS.md -->

# 11. Current state, limitations, and discrepancies

## Dated snapshot

The following observations were recorded during the curated-storefront release on **2026-11-05**. They are not permanent health claims. Reconfirm before operating.

- AHSANLY is live at `ahsanli.com`; Egypt serves `/ar` and `/en`.
- Active source commit for the curated release: `cbbf57a74e980ea28ad6e4dac1145c5fef03e3b8`.
- Go API, Next web, worker, and pipeline were active after automatic deployment.
- Public Arabic, English, archive, and console routes returned HTTP 200.
- No candidate was automatically approved to populate the new design.
- The public approved-deals response was empty, so the honest empty state was intentional and verified.
- An uncurated public product URL returned 404.
- No production publication was manufactured for the release.

## Amazon source state and cap discrepancy

At that snapshot, the `amazon_eg` retailer reported `health=blocked`. Its stored `max_requests_per_hour` value was **480**, while older operational guides described a conservative **120 requests/hour** cap. This is a real documentation/live-data discrepancy discovered during verification.

The feature work did not change the value. Do not silently normalize it in a future UI/docs task. A future operator should inspect actual worker attempt counts, source health, and the intended policy, then make an explicit scoped decision. The six-hour blocked-source behavior remains in the worker claim query.

## Approved-deal scarcity

The new business model deliberately trades volume for selection. The home page may contain only a featured deal, only a few latest deals, or none. This is not an API failure. `null`/unavailable API behavior and a valid empty array have separate UI states.

When Amazon is blocked or no candidate has been reviewed, no fresh deals should appear. Do not fill the page from the legacy catalog or imported prices.

## Evidence availability

The only preexisting candidate observed at the curated migration was `held`; there was no need to auto-review it. Existing publication history and deduplication keys are preserved by design. A candidate being captured, old, imported, or present in the admin list is not sufficient for public visibility.

## Legacy code and documentation

Historical files may mention:

- Koupnak or BotAfflite branding.
- Old service names, ports, watchlist sizes, or `/root/botAfflite` paths.
- Automatic publication of captured candidates before the editorial gate.
- Active Windows collectors or legacy Django services.
- Older source caps and monitoring coverage.

These are historical records, not instructions for the current stack. The active services are `ahsanly-api`, `ahsanly-web`, `ahsanly-worker`, and `ahsanly-pipeline`. Legacy services remain stopped/disabled and must not be started for routine operation.

## Technical limitations

- The 1 GB VPS is not used for routine builds; artifacts come from GitHub.
- Only Egypt is publicly launched; other countries require a full scoped launch.
- Noon/Jumia remain paused/blocked or unsupported for fresh direct publication.
- Category discovery exists but is not continuously scheduled.
- Public price charts were removed from the sales flow, although observation history remains stored.
- The current CI workflow runs unit/build checks; stateful PostgreSQL integration tests require a separately prepared isolated database.
- GitHub release polling is near-real-time, not an instantaneous webhook.
- Database backups should not be assumed to have an off-server destination.

## What not to infer

- Catalog size does not imply current monitoring coverage.
- A stored observation does not imply a current offer.
- A source success timestamp does not necessarily imply a valid price-and-seller observation.
- A Telegram sent message does not imply that the offer is still fresh today.
- A successful deployment does not imply that source health recovered.
- A documented limit does not override contradictory live data; reconfirm and report it.

---

<!-- Source: docs/12_AI_HANDOFF.md -->

# 12. AI handoff and review checklist

## Purpose of the standalone guide

`AHSANLY_AI_GUIDE.md` is generated from all sections in this directory. It is designed to be the only file an AI needs for an initial understanding of the project. It contains product rules, architecture, routes, API, data relationships, publication state machines, security, testing, deployment, known limitations, and safe extension recipes.

It does not contain secrets, customer data, database dumps, or evidence images. It does not replace inspecting current code or live operational state.

## Recommended AI workflow

1. Read the standalone guide completely before proposing changes.
2. Identify whether the request concerns product policy, frontend, public API, database, worker, editorial review, Telegram, deployment, or documentation.
3. Inspect the exact current source files listed in the repository map.
4. Read `AGENTS.md` and `ops/SERVER.md` for operational commands and any newer notes.
5. Reconfirm live services, active commit, source health, queue/publication state, backups, and memory when production behavior matters.
6. Distinguish current behavior from dated/historical notes.
7. State assumptions and preserve invariants before editing.
8. Implement with the project's existing Go/Next idioms and dependencies; do not introduce a new framework by default.
9. Add/update tests in existing test locations and update documentation when behavior changes.
10. Run the appropriate local/isolated checks, regenerate the guide, and verify automatic deployment and production health.

## Non-negotiable product invariants

- The public product is curated real deals, not catalog volume or public price comparison.
- Human approval is required before website/Telegram publication.
- Store-displayed discount and AHSANLY historical drop remain separate facts.
- Imported, stale, wrong-country, non-current, unreviewed, rejected, or evidence-mismatched data cannot become a current deal.
- No cheapest-across-stores claim without independent evidence; the current system does not make that claim.
- Empty states are honest and acceptable.
- Product titles remain retailer-supplied; interface content supports Arabic and English.

## Non-negotiable operational invariants

- One active Go worker and one active Go pipeline; no legacy concurrent writer.
- Photo-only Telegram deals with fresh direct matching evidence.
- Preserve IDs, observations, publication keys, Telegram message IDs, and audit history.
- No secrets in chat, source, logs, screenshots, or documentation.
- No Windows project task reactivation without a new explicit request.
- No source-cap/backoff changes or new retailers as an unrelated side effect.
- No old database restore over newer observations.
- No fabricated live deal or Telegram post for a smoke test.

## Before accepting a future change

Ask/verify from code and environment:

- Does it accidentally bypass editorial review or another gate?
- Does it confuse merchant discount with measured historical drop?
- Does it expose uncurated products publicly?
- Does it break country or language isolation?
- Does it create cursor duplicates or stale deal leakage?
- Does it change migration compatibility or publication deduplication?
- Does it add another writer, excessive memory, or VPS build load?
- Does it rely on an outdated operational statement?
- Are Go tests, isolated DB tests where relevant, Next build/browser checks, and docs freshness all covered?

## Documentation maintenance

Edit the relevant numbered source section, not only the generated file. Run:

```bash
python docs/build_docs.py
python docs/build_docs.py --check
```

Commit both the source sections and generated `AHSANLY_AI_GUIDE.md`. CI rejects a stale generated guide. For a future AI, download/share the generated file directly from the repository root.
