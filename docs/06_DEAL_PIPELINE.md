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
