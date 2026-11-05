# Full Go conversion: runbook and current boundary

Last updated: 2026-11-04. Recheck the VPS before acting; service and source health change.

**Latest source-rate status (16:15 UTC):** The VPS now marks 63,060 Amazon listings as Go-owned and the legacy publisher is inactive/disabled, superseding the 282-listing split below. The 480/hour setting produced at most 135 valid offers in observed one-hour windows and led to source health `blocked` at 14:29 UTC. Go now has a 120/hour provisional cap, verified TLS and transport-consistent ALPN. Its automatic six-hour source backoff remains in place. See [SERVER.md](SERVER.md) for backup, rollback and post-cooldown measurements. The large catalog is not fresh merely because it was transferred.

## Retained old version

The complete pre-conversion local workspace is preserved at
`C:\Users\AbdelrahmanMostafa\Desktop\botAfflite-OLD-before-full-Go-2026-11-03`.
It contains 23,975 files (1,790.8 MB) including data and secrets. Key source
hashes were compared with the active workspace at snapshot time. Its
`README_OLD_VERSION.md` explains its purpose. **Do not delete, edit, upload,
or sync this folder.** Keep working in `C:\Users\AbdelrahmanMostafa\Desktop\botAfflite`.

## Current architecture

| Function | Current owner | Go conversion |
| --- | --- | --- |
| Public catalog/search/product/history API | Go service `botafflite-go-api.service` | Live, read-only |
| Public Arabic site | Next.js service `botafflite-next-preview.service` | Live |
| Protected dashboard, staff login and TOTP | Go API + Next `/console`; Django fallback | Go password/TOTP/session/CSRF and match review live; Django still handles first-time TOTP setup |
| Selected Amazon offer queue | `botafflite-go-worker.service` | Live candidate worker for 282 mobile/pilot ASINs; Django worker stopped and disabled |
| Amazon category pagination | No active collector | Go discovery implemented but not scheduled |
| Exact-variant matching/review | Django suggestions; Go review UI | Go staff can approve/reject proposed matches with audit records |
| Screenshot evidence for a candidate | Go `botafflite-go-pipeline.service` | Opens Chromium only for a qualifying direct drop |
| Telegram photo outbox | Go `botafflite-go-pipeline.service` | Live for verified Go-owned Amazon candidates |
| Other Amazon products | Legacy `amazon-tracker.service` | Publishes only nonoverlapping products until migration is complete |
| Noon/Jumia direct monitoring | Unreliable/blocked from VPS | No Go connector enabled |
| Database | PostgreSQL 18 | Shared schema; Go uses pgx |

No BotAfflite task may run persistently on the Windows PC. The four previously
named Windows Scheduled Tasks are disabled. Do not use the PC as a fetch proxy.

## Database and rollback point

Before migrations 0013 and 0014, a VPS-local PostgreSQL backup was completed.
Before the Go admin session migration, another VPS-local dump completed at
`/root/botAfflite/data/pg_backups/botafflite-20261103-175821.dump`.
Backups stay on that VPS and are not disaster recovery for VPS loss.
Migration 0013 adds `DealCandidate.capture_lease_until` and
`Publication.delivery_lease_until`; migration 0014 adds
`Publication.next_attempt_at`. The live Django/legacy services remained active
after both migrations. The server's previous `models.py` is saved as
`models.py.before-go13` and `models.py.before-go14` in its code directory.
Go migration `0001_admin_session.sql` adds only `go_admin_session` and its
migration ledger. It is live and does not change Django sessions.

The local migration check used an isolated SQLite database at
`go_platform/out/migration_check.sqlite3`. It did not modify `deal_dev.sqlite3`.
Do not restore the PostgreSQL dump over the running database merely to roll
back Go. Stop the Go shadow worker before restarting the retained Django
worker, and revert an API binary or service independently. A full restore
discards newer observations.

## Go workflow and safety gates

1. Offer and category jobs use PostgreSQL row leases and `SKIP LOCKED`. The Go
   worker defaults to `shadow` candidates; it never publishes by default.
2. `cmd/worker --live-candidates` requires `--apply`, both
   `DEAL_PUBLISH_ENABLED=1` and `DEAL_GO_PUBLISH_ENABLED=1`, and the retailer's
   `publish_enabled` gate before producing a publishable candidate.
3. `cmd/worker --capture --once` requires the same environment gates and a
   qualifying fresh Amazon candidate. Chromium opens only at this stage. A
   PostgreSQL advisory lock restricts it to one capture at a time. Screenshot
   price and seller must match the latest direct observation; a failure is
   retried up to three times and then held. Use `cmd/capture` for read-only
   diagnostics without candidate writes.
4. `cmd/publish --prepare` creates deduplicated publication rows for eligible
   routes and verified affiliate links. `--send` rechecks the candidate,
   source, route, price, seller, screenshot file and caption before a photo
   upload. An uncertain Telegram result is held for manual review and never
   blindly resent; an explicit 429 can retry after 15 minutes. The command
   defaults to private test channels; the public channel needs the explicit
   `--allow-live-channels` flag after cutover.
5. Any stale/blocked/missing source or missing photo produces no post. Imported
   Kanbkam hints are never direct offers. No cross-store best-price claim is
   used: the rule compares each listing with its immediately preceding direct
   in-stock observation and uses the country percentage threshold.

The Go **live offer worker** runs as `botafflite-go-worker.service`; the Django
worker is stopped and disabled. The selected watchlist has 282 Amazon ASINs:
16 hourly and 266 daily. A separate `botafflite-go-pipeline.service` captures
and sends qualifying verified photos. `cmd/testphoto` sent one clearly labelled
real product-page photo to Koupnak on 2026-11-04 (Telegram message 1830), with
no discount claim. No synthetic price drop was introduced at cutover. The
legacy service continues for nonoverlapping products. Do not start the Django
worker concurrently with the Go worker on the shared queue.

The reviewed Go source is staged on the VPS under
`/root/botAfflite/go_platform/{cmd,internal,deploy,go.mod,go.sum}`. The Linux
worker executable is `/root/botAfflite/go_platform/bin/botafflite-worker` with
unit `/etc/systemd/system/botafflite-go-worker.service`. The publisher remains
a dormant candidate file `go_platform/bin/botafflite-publish.candidate-20261103`;
there is no publisher unit. The original source archive is
`data/go-migration-stage-20261103/go-source-20261103.tar.gz`. Uploaded archives
and executables were checked against local SHA-256 hashes. `--help` runs on
the VPS, and both publication gates rejected a command without those gates.

The Go API now serves protected `/api/v1/admin/*` routes. It verifies an
existing active Django staff password and confirmed TOTP device, prevents TOTP
replay, uses an eight-hour Secure/HttpOnly/SameSite session cookie and checks
Origin plus CSRF on mutations. Nginx limits login POSTs. Next `/console` offers
status, watched listings, rule thresholds, channels, match reviews, candidates
and publications; mutations add `AuditEvent` rows. The existing Django admin
remains the fallback and TOTP enrollment path. Isolated PostgreSQL tests passed
for baseline/drop, route fan-out, uncertain delivery, admin login/logout/CSRF,
watchlist/rule edits and exact-match review. The real staff password was not
used in automated testing.

The Go console now shows countries, Telegram destinations and per-country or
per-retailer publication routes. Staff can create a disabled Telegram channel
using a token environment-variable name and create a disabled route, then
enable/pause both with audited changes. An isolated PostgreSQL test verifies
duplicate route prevention and that channel activation requires an existing
server token reference. Non-Telegram transports and country creation still use
Django admin. This does not enable the dormant Go photo publisher.

The user explicitly accepted the already exposed token and waived the 48-hour
OOM coverage wait. The normal release gate retains both checks. For this
cutover, `release_gate --retailer amazon_eg --allow-public-photo-test
--accept-existing-token --skip-48h-oom` passed reconciliation, recent
48-hour source pilot at 95% over 20 URLs, verified affiliate link, labelled
photo test, matching current Go/legacy token settings, no uncertain test
delivery and a recent zero-OOM audit. The pilot gate uses a rolling 72-hour
report window. The initial token fingerprint remains root-only at
`data/pilot/legacy_token_before_rotation.sha256`; never display the token.

The 2026-11-04 PostgreSQL backup is
`data/pg_backups/botafflite-20261104-023343.dump` and the consistent legacy
SQLite backup is `data/legacy_backups/tracker-20261104T023343Z.sqlite3`.
`manage.py cutover_mobile_watchlist --apply` prepared 282 watch jobs and
affiliate links. The legacy service was stopped while `--retire-legacy`
paused its 237 overlapping ASIN rows, then restarted for the remaining
products. `--enable-go` verified no overlap, marked the 282 selected listings
`extracted_attributes.go_owned=true`, and opened the Amazon retailer gate.
Live Go job claim, capture and outbox SQL require this marker; isolated
PostgreSQL tests rejected an unowned listing. The old service still tracks
62,771 other products. Both Go environment flags are on; a photo is sent only for a
fresh direct drop of at least 10% with matching screenshot evidence. The
Go photo service uses `/opt/botafflite-chromium/chrome-headless-shell` and
`deal_captures/`; a read-only capture as `botdeals` passed under a 500 MB cap.
Rollback steps are at the top of [SERVER.md](SERVER.md).

## Release sequence

1. Verify source permissions and affiliate examples for each retailer. Amazon
   can be piloted; Noon/Jumia need written collection scope, rate limits,
   verified link formats, and a reliable VPS price-and-seller source. A library
   alone cannot turn blocked pages into verified offers.
2. Continue Go integration coverage for lease contention, restart recovery,
   screenshot failure and stale observations. Monitor memory on the 1 GB VPS.
3. Complete Go category scheduling, TOTP enrollment, channel/route editing,
   source pilots, backup/summary jobs, and other management workflows before
   retiring Django. Retain staff authorization and audit events.
4. The existing Koupnak photo test (message 1830) passed. The user waived token
   rotation and the 48-hour OOM coverage for this cutover; record these explicit
   options when rerunning `release_gate`. The default gate still requires both.
   Rotate the token later through BotFather if the user changes this decision.
5. Keep the 282 selected ASINs exclusively in Go while the legacy service
   handles the rest. Review source success, capture failures, uncertain sends,
   memory and the first natural deal publication. Expand Go ownership only with
   an explicit bounded schedule and a corresponding legacy pause. At 30 source
   requests per hour, moving all ~63,000 old products to fresh Go monitoring
   at once would take roughly 87 days for one pass. Never publish the same
   product/channel from both services.
6. Roll back a failed cutover by disabling the two publication gates and Go
   publisher first; reconcile uncertain Telegram sends and latest message IDs;
   only then restore the legacy publisher. Leave the preserved local snapshot
   and PostgreSQL history intact.

Commands and current service paths are in [SERVER.md](SERVER.md). Do not put
the SSH key, bot token, `deal.env`, `settings.json`, or `tracker_config.json`
into an archive, log, documentation, or Go build artifact.

Shadow-worker rollback: stop and disable `botafflite-go-worker.service`, then
enable and start `botafflite-worker.service`. Do not run both at once. The
current Go admin API binary backup is
`go_platform/bin/botafflite-api.before-channels-20261103`; the Next backup
is `/root/botAfflite/web_next.before-channels-20261103`. Nginx config backup
is `/etc/nginx/sites-available/botafflite.before-go-admin-20261103`.
