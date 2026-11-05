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
