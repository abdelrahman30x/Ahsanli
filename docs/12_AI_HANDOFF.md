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
