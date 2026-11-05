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
