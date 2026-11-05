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
