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
