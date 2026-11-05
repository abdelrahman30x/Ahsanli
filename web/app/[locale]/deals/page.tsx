import type { Metadata } from "next";
import Link from "next/link";
import ProductCard from "@/components/ProductCard";
import { listProducts } from "@/lib/api";
import { Locale, t, direction } from "@/lib/i18n";

export async function generateMetadata({ params }: { params: Promise<{ locale: Locale }> }): Promise<Metadata> {
  const { locale } = await params;
  return {
    title: `${t(locale, 'navDeals')} | ${t(locale, 'brand')}`,
    description: t(locale, 'dealsIntro'),
  };
}

export default async function Deals({ params, searchParams }: { params: Promise<{ locale: Locale }>; searchParams: Promise<{ cursor?: string }> }) {
  const { locale } = await params;
  const raw = await searchParams;
  const cursor = typeof raw.cursor === 'string' ? raw.cursor : '';
  const country = 'EG';
  const deals = await listProducts(locale, country, { sort: "drop", verified: "1", cursor, limit: "24" });
  const dir = direction(locale);
  const nextParams = new URLSearchParams();
  if (deals?.next_cursor) nextParams.set('cursor', deals.next_cursor);

  return <main className="catalog-section standalone-page">
    <div className="section-heading">
      <div>
        <span className="section-kicker">{t(locale, 'dealsKicker')}</span>
        <h1>{t(locale, 'dealsTitle')}</h1>
        <p>{t(locale, 'dealsIntro')}</p>
      </div>
      <span className="section-mark">/ DEALS</span>
    </div>
    {!deals ? <div className="empty-state"><strong>{t(locale, 'catalogUnavailable')}</strong></div> :
     deals.items.length === 0 ? <div className="empty-state"><strong>{t(locale, 'noDeals')}</strong><p>{t(locale, 'noDealsHint')}</p></div> :
     <>
      <div className="product-grid">{deals.items.map(p => <ProductCard key={p.id} product={p} locale={locale} />)}</div>
      <div className="pagination">
        {deals.has_more && <Link href={`/${locale}/deals?${nextParams}`}>{t(locale, 'next')} {dir === 'rtl' ? '←' : '→'}</Link>}
      </div>
     </>}
  </main>;
}
