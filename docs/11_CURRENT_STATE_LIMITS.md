# 11. Current state, limitations, and discrepancies

## Dated snapshot

The following observations were recorded during the curated-storefront release on **2026-11-05**. They are not permanent health claims. Reconfirm before operating.

- AHSANLY is live at `ahsanli.com`, serving Egypt and eight additional Middle East country subdomains (`eg.`, `sa.`, `ae.`, `kw.`, `jo.`, `om.`, `bh.`, `qa.`, `iq.ahsanli.com`).
- Let's Encrypt TLS certificate covers all 11 apex, www, and country subdomains.
- Go API, Next web, worker, and pipeline are active on the VPS.
- Public Arabic, English, archive, and console routes return HTTP 200 across all subdomains.
- No candidate was automatically approved to populate the new design; unpopulated countries display honest empty states.
- An uncurated public product URL returns 404.
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
- Egypt has active retailer monitoring; other country subdomains are enabled with honest empty states awaiting dedicated retailer integrations.
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
