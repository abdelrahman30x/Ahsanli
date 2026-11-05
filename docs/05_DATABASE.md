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

Nine Middle East countries are configured with public storefront hostnames and enabled public site gates: Egypt (`eg.ahsanli.com`), Saudi Arabia (`sa.ahsanli.com`), Jordan (`jo.ahsanli.com`), UAE (`ae.ahsanli.com`), Kuwait (`kw.ahsanli.com`), Oman (`om.ahsanli.com`), Bahrain (`bh.ahsanli.com`), Qatar (`qa.ahsanli.com`), and Iraq (`iq.ahsanli.com`). Each country has a dedicated unique deal rule in `core_dealrule`.

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

Migrations are embedded with `go:embed`, sorted by filename, and applied transactionally under a PostgreSQL advisory lock. `go_schema_migrations` records names. Use a new ordered file for every change; never edit an already deployed migration. Prefer additive, backward-compatible changes so code rollback against the same newer database remains safe. Active migrations include:
- `0001_admin_session.sql`: Session table and indices.
- `0002_country_hostname.sql`: Hostname, timezone, and locale fields on `core_country`.
- `0003_channel_public_url.sql`: Public URL on publication channels.
- `0004_category_taxonomy.sql`: Curated category taxonomy.
- `0005_editorial_deal_review.sql`: Editorial candidate review fields and review queue indices.
- `0006_additional_countries.sql`: Seed Middle East countries with dedicated subdomains and unique country deal rules.
