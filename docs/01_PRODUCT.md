# 1. Product identity and business rules

## What AHSANLY is

AHSANLY (Arabic: **أحسنلي**) is a multi-country, bilingual, curated-deals platform. Its tagline is **“أحسن عرض.. أحسن سعر”**. The public product is a small selection of worthwhile, verified store offers—not a large catalog, a general search engine, or a shopper-facing price-comparison tool.

Egypt is the primary operating storefront with live observations and curated deals. The platform supports nine Middle East countries with active subdomains and localized `/ar/` (right-to-left) and `/en/` (left-to-right) routes:

- Egypt: `https://eg.ahsanli.com/` (`EGP`)
- Saudi Arabia: `https://sa.ahsanli.com/` (`SAR`)
- Jordan: `https://jo.ahsanli.com/` (`JOD`)
- United Arab Emirates: `https://ae.ahsanli.com/` (`AED`)
- Kuwait: `https://kw.ahsanli.com/` (`KWD`)
- Oman: `https://om.ahsanli.com/` (`OMR`)
- Bahrain: `https://bh.ahsanli.com/` (`BHD`)
- Qatar: `https://qa.ahsanli.com/` (`QAR`)
- Iraq: `https://iq.ahsanli.com/` (`IQD`)

In addition:
- `https://ahsanli.com/` — apex entry that uses a saved country preference or local IP geolocation.
- `https://ahsanli.com/countries` — permanent country selector with flags and currency metadata.
- `https://ahsanli.com/console` — the only staff origin.

The purchased domain is spelled `ahsanli.com`; the brand spelling remains **AHSANLY** even though the hostname uses `ahsanli`.

## The customer promise

A visitor should be able to answer:

1. What is the product and its exact retailer listing?
2. What is its current item price and currency?
3. Which store and seller provide it?
4. When was it last checked successfully?
5. Is the percentage a discount displayed by the store or a historical drop measured by AHSANLY?
6. Why did AHSANLY select it?
7. How do I open the store page?

The platform may earn affiliate commission from eligible links. Shipping is excluded from the observed item price, and the customer is told to verify variant, seller, shipping, availability, and final price at the store.

## Editorial responsibility

Not every lower price is a deal worth publishing. Automated monitoring creates evidence and candidates; **AHSANLY staff make the publication decision**. Human approval is required for both website visibility and Telegram publication. A future ranking system may prioritize the review queue, but it must not silently become an automatic approval mechanism.

The platform does not claim that an item is the cheapest across stores. Historical comparison is an internal evidence and selection tool. Public copy focuses on the selected deal.

## Two different discount facts

Never merge or confuse these fields:

- **Store-displayed discount**: a percentage independently seen on the retailer page during evidence capture. Public label: “Store-displayed discount” / “خصم المتجر المعروض”.
- **AHSANLY historical drop**: the percentage difference between the current direct observation and the immediately previous valid direct in-stock observation for the same listing. Public label: “Verified price drop” / “انخفاض سعر موثق”.

If the store displayed no discount, do not invent one. If only a historical drop exists, do not describe it as a retailer promotion. A stored/imported price cannot be used as proof of a current deal.

## Public content rules

Only deals satisfying all current gates appear publicly:

- Correct enabled country and public-site gate.
- Enabled retailer with publication enabled.
- Go-owned listing.
- Fresh, direct, non-imported, in-stock observation.
- Current-offer pointer still references the winning observation.
- Matching screenshot path, price, and seller.
- Current rule version and enabled deal rule.
- Editorial state `approved` or previously queued/sent history that remains currently eligible.

Uncurated product IDs return 404 in the new public Next experience. An empty approved-deal list is valid and preferable to filler. The empty state promotes the country Telegram channel without fabricating discounts.

## Non-goals for the current product

- Shopper accounts, wishlists, saved searches, or personal alerts.
- Public cross-store comparison tables.
- Maximizing product count or claiming comprehensive price freshness.
- Publishing every qualifying percentage drop.
- Automatic editorial approval.
- Treating category/search-result prices as direct product evidence.
- Enabling Noon, Jumia, or another country merely because legacy rows exist.

## Brand and accessibility

The interface uses a warm off-white background, deep navy text, blue actions, amber commercial highlights, and green reserved for verified states. IBM Plex Sans Arabic and Manrope are self-hosted. The layout is mobile-first, keyboard navigable, and intended to meet WCAG AA contrast. Public pages remain primarily server-rendered; no large client-side UI framework is needed on the 1 GB VPS.
