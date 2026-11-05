import type { Metadata } from "next";
import Link from "next/link";
import DealCard from "@/components/DealCard";
import { getCountryMeta, listDeals } from "@/lib/api";
import { resolveCurrentCountry } from "@/lib/country";
import { Locale, t, direction } from "@/lib/i18n";

type SearchParams = Record<string, string | string[] | undefined>;
function one(value: string | string[] | undefined) { return typeof value === "string" ? value : ""; }

export async function generateMetadata({ params }: { params: Promise<{ locale: Locale }> }): Promise<Metadata> {
  const { locale } = await params;
  const resolved = await resolveCurrentCountry();
  return {
    title: `${t(locale, 'navDeals')} | ${t(locale, 'brand')}`,
    description: t(locale, 'dealsIntro'),
    alternates: {
      canonical: `https://${resolved.hostname}/${locale}/deals`,
      languages: {
        [`ar-${resolved.code}`]: `https://${resolved.hostname}/ar/deals`,
        en: `https://${resolved.hostname}/en/deals`
      }
    },
  };
}

export default async function Deals({ params, searchParams }: { params: Promise<{ locale: Locale }>; searchParams: Promise<SearchParams> }) {
  const { locale } = await params;
  const raw = await searchParams;
  const resolved = await resolveCurrentCountry();
  const country = resolved.code;
  const store = one(raw.store), category = one(raw.category), cursor = one(raw.cursor);
  const query: Record<string, string> = { limit: "24" };
  if (store) query.store = store;
  if (category) query.category = category;
  if (cursor) query.cursor = cursor;
  const [deals, countryMeta] = await Promise.all([
    listDeals(country, query),
    resolved.meta ? Promise.resolve(resolved.meta) : getCountryMeta(country),
  ]);
  const stores = (countryMeta?.retailers || []).filter(item => item.enabled);
  const categories = countryMeta?.categories || [];
  const dir = direction(locale);
  const nextParams = new URLSearchParams();
  if (store) nextParams.set('store', store);
  if (category) nextParams.set('category', category);
  if (deals?.next_cursor) nextParams.set('cursor', deals.next_cursor);

  return <main className="deals-archive">
    <section className="archive-heading">
      <span className="section-kicker">{t(locale, 'dealsKicker')}</span>
      <h1>{t(locale, 'dealsTitle')}</h1>
      <p>{t(locale, 'dealsIntro')}</p>
      {countryMeta?.telegram_url && <a className="button-primary" href={countryMeta.telegram_url} target="_blank" rel="noopener noreferrer">{t(locale, 'telegramCta')} ↗</a>}
    </section>

    <nav className="deal-chip-groups" aria-label={t(locale, 'mainNavigation')}>
      <div className="deal-chips" aria-label={t(locale, 'filterStore')}>
        <Link className={!store ? 'active' : ''} href={category ? `/${locale}/deals?category=${category}` : `/${locale}/deals`}>{t(locale, 'allStores')}</Link>
        {stores.map(item => <Link key={item.code} className={store === item.code ? 'active' : ''} href={`/${locale}/deals?store=${item.code}${category ? `&category=${category}` : ''}`}>{item.name}</Link>)}
      </div>
      {categories.length > 0 && <div className="deal-chips" aria-label={t(locale, 'filterCategory')}>
        <Link className={!category ? 'active' : ''} href={store ? `/${locale}/deals?store=${store}` : `/${locale}/deals`}>{t(locale, 'allCategories')}</Link>
        {categories.map(item => <Link key={item.code} className={category === item.code ? 'active' : ''} href={`/${locale}/deals?category=${item.code}${store ? `&store=${store}` : ''}`}>{locale === 'ar' ? item.name_ar : item.name_en}</Link>)}
      </div>}
    </nav>

    {!deals ? <div className="empty-state"><strong>{t(locale, 'catalogUnavailable')}</strong></div> : deals.items.length === 0 ? <div className="empty-state deals-empty"><span className="empty-tag" aria-hidden="true">%</span><strong>{t(locale, 'noDeals')}</strong><p>{t(locale, 'noDealsHint')}</p></div> : <>
      <div className="deal-grid">{deals.items.map(deal => <DealCard key={deal.deal_id} deal={deal} locale={locale} />)}</div>
      <div className="pagination">{deals.has_more && <Link className="button-secondary" href={`/${locale}/deals?${nextParams}`}>{t(locale, 'next')} {dir === 'rtl' ? '←' : '→'}</Link>}</div>
    </>}
  </main>;
}
