import Link from "next/link";
import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { dateTime, getDeal, money } from "@/lib/api";
import { resolveCurrentCountry } from "@/lib/country";
import { Locale, t } from "@/lib/i18n";

export async function generateMetadata({ params }: { params: Promise<{ locale: Locale; id: string }> }): Promise<Metadata> {
  const { locale, id } = await params;
  const resolved = await resolveCurrentCountry();
  const deal = await getDeal(resolved.code, id);
  if (!deal) return { title: `${t(locale, 'productNotFound')} | ${t(locale, 'brand')}`, robots: { index: false } };
  const description = `${t(locale, 'verifiedPrice')}: ${money(deal.price, deal.currency, locale)} — ${t(locale, 'storeCaution')}`;
  return {
    title: `${deal.title} | ${deal.store_name} | ${t(locale, 'brand')}`,
    description,
    alternates: {
      canonical: `https://${resolved.hostname}/${locale}/products/${deal.id}`,
      languages: {
        [`ar-${resolved.code}`]: `https://${resolved.hostname}/ar/products/${deal.id}`,
        en: `https://${resolved.hostname}/en/products/${deal.id}`,
      },
    },
    openGraph: {
      title: deal.title,
      description,
      url: `https://${resolved.hostname}/${locale}/products/${deal.id}`,
      siteName: t(locale, 'brand'),
      locale: locale === 'ar' ? 'ar_EG' : 'en_US',
      type: 'website',
    },
  };
}

export default async function ProductPage({ params }: { params: Promise<{ locale: Locale; id: string }> }) {
  const { locale, id } = await params;
  const resolved = await resolveCurrentCountry();
  const deal = await getDeal(resolved.code, id);
  if (!deal) notFound();
  const note = locale === 'ar' ? deal.editorial_note_ar : deal.editorial_note_en;

  return <main className="detail-main">
    <div className="breadcrumbs"><Link href={`/${locale}/deals`}>{t(locale, 'navDeals')}</Link><span>/</span><span>{deal.store_name}</span><span>/</span><span>{deal.product_id}</span></div>
    <section className="detail-hero">
      <div className="detail-media">
        {deal.image_url ? <img src={`/api/proxy-image?url=${encodeURIComponent(deal.image_url)}`} alt={deal.title} /> : <div className="detail-placeholder">{deal.store_name.charAt(0)}</div>}
        <span className="detail-verified"><span aria-hidden="true">✓</span>{t(locale, 'verifiedDeal')}</span>
      </div>
      <div className="detail-copy">
        <div className="detail-tags"><span>{deal.store_name}</span><span className="status fresh">{t(locale, 'verifiedPrice')}</span></div>
        <h1>{deal.title}</h1>
        <div className="detail-price-label">{t(locale, 'verifiedPrice')}</div>
        <div className="detail-price">{money(deal.price, deal.currency, locale)}</div>
        {deal.displayed_discount && <div className="merchant-discount"><span>{t(locale, 'storeDiscount')}</span><strong>{Number(deal.displayed_discount).toFixed(0)}%</strong><small>{locale === 'ar' ? 'كما ظهر في صفحة المتجر وقت التحقق' : 'As displayed on the store page at verification time'}</small></div>}
        {deal.drop_percent && <div className="historical-fact"><span>{t(locale, 'verifiedDrop')}</span><strong>{Number(deal.drop_percent).toFixed(1)}%</strong><small>{locale === 'ar' ? 'مقارنة بآخر قراءة مباشرة سابقة لدى أحسنلي؛ ليس خصمًا معلنًا من المتجر' : 'Compared with AHSANLY’s previous direct reading; not a store-displayed discount'}</small></div>}
        {note && <div className="editorial-note"><span>{t(locale, 'editorialNote')}</span><p>{note}</p></div>}
        <div className="detail-facts">
          <div><span>{locale === 'ar' ? 'البائع' : 'Seller'}</span><strong>{deal.seller || t(locale, 'sellerUnknown')}</strong></div>
          <div><span>{t(locale, 'lastChecked')}</span><strong>{dateTime(deal.observed_at, locale)}</strong></div>
          <div><span>{locale === 'ar' ? 'المتجر' : 'Store'}</span><strong>{deal.store_name}</strong></div>
        </div>
        {deal.affiliate_url.startsWith("https://") ? <a className="button-primary buy-button" href={deal.affiliate_url} target="_blank" rel="noopener noreferrer sponsored">{t(locale, 'openStore')} ↗</a> : <div className="link-pending">{t(locale, 'linkPending')}</div>}
        <p className="detail-caution">{t(locale, 'storeCaution')}</p>
      </div>
    </section>
    <section className="deal-proof">
      <div><span>01</span><strong>{t(locale, 'howObserveTitle')}</strong><p>{t(locale, 'howObserveBody')}</p></div>
      <div><span>02</span><strong>{t(locale, 'howCompareTitle')}</strong><p>{t(locale, 'howCompareBody')}</p></div>
      <div><span>03</span><strong>{t(locale, 'howDecideTitle')}</strong><p>{t(locale, 'howDecideBody')}</p></div>
    </section>
  </main>;
}
