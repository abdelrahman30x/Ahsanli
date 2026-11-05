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

## Country switching and localization

`web/lib/i18n.ts` defines `ar` and `en`, direction, and all shared interface strings. Add new keys to the Arabic object first and mirror them in the typed English object. Do not translate retailer-supplied product titles.

The country switcher dropdown (`web/components/CountrySwitcher.tsx`) displays the active country flag and triggers a dropdown with all supported Middle East countries and their dedicated subdomains (EG `eg.`, SA `sa.`, JO `jo.`, AE `ae.`, KW `kw.`, OM `om.`, BH `bh.`, QA `qa.`, IQ `iq.`). The language switcher preserves the current path and query while replacing only the leading locale. The mobile drawer contains deals, stores, selection policy, about, Telegram, and country switching with flag previews. Closed drawer content is removed from display to avoid off-screen overflow and keyboard focus.

## Metadata and sharing

Localized layouts emit canonical URLs and reciprocal `ar-{COUNTRY}`/`en` hreflang links based on the active country subdomain. Deals and deal detail pages add their own canonical/alternate links; deal detail also emits Open Graph metadata. Only currently public approved deals should be indexed or included in future sitemap additions. Missing/expired deals return noindex/not-found behavior rather than presenting stale content as current.

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
