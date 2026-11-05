import Link from "next/link";
import { notFound } from "next/navigation";
import type { Metadata } from "next";
import PriceChart from "@/components/PriceChart";
import { dateTime, getProduct, money } from "@/lib/api";
import { Locale, t } from "@/lib/i18n";

export async function generateMetadata({ params }: { params: Promise<{ locale: Locale; id: string }> }): Promise<Metadata> {
  const { locale, id } = await params;
  const detail = await getProduct('EG', id);
  if (!detail) return { title: `${t(locale, 'productNotFound')} | ${t(locale, 'brand')}`, robots: { index: false } };
  const p = detail.product;
  return {
    title: `${p.title} | ${p.store_name} | ${t(locale, 'brand')}`,
    description: `${p.imported ? t(locale, 'importedPrice') : t(locale, 'verifiedPrice')}: ${money(p.price, p.currency, locale)} — ${t(locale, 'storeCaution')}`,
    robots: { index: p.fresh, follow: true },
  };
}

export default async function ProductPage({ params }: { params: Promise<{ locale: Locale; id: string }> }) {
  const { locale, id } = await params;
  const detail = await getProduct('EG', id);
  if (!detail) notFound();
  const p = detail.product;
  const direct = detail.history.filter(x => !x.imported);
  const prices = direct.map(x => Number(x.price));
  const min = prices.length ? Math.min(...prices) : null;
  const max = prices.length ? Math.max(...prices) : null;
  const statusClass = p.fresh ? "status fresh" : p.imported ? "status imported" : "status stale";
  const statusText = p.fresh ? t(locale, 'verifiedPrice') : p.imported ? t(locale, 'importedPrice') : t(locale, 'stalePrice');

  return <main className="detail-main">
    <div className="breadcrumbs">
      <Link href={`/${locale}/`}>{t(locale, 'navProducts')}</Link>
      <span>/</span>
      <span>{p.store_name}</span>
      <span>/</span>
      <span>{p.product_id}</span>
    </div>
    <section className="detail-hero">
      <div className="detail-media">
        {p.image_url ? <img src={`/api/proxy-image?url=${encodeURIComponent(p.image_url)}`} alt={p.title} /> : <div className="detail-placeholder">{p.store === "amazon_eg" ? "A" : p.store === "noon_eg" ? "N" : "J"}</div>}
      </div>
      <div className="detail-copy">
        <div className="detail-tags">
          <span>{p.store_name}</span>
          <span className={statusClass}>{statusText}</span>
        </div>
        <h1>{p.title}</h1>
        <div className="detail-price-label">{statusText}</div>
        <div className="detail-price">{money(p.price, p.currency, locale)}</div>
        {p.drop_percent && p.fresh && <div className="detail-drop">↓ {t(locale, 'dropLabel')} {Number(p.drop_percent).toFixed(1)}%</div>}
        <div className="detail-facts">
          <div><span>{locale === 'ar' ? 'البائع' : 'Seller'}</span><strong>{!p.imported && p.seller ? p.seller : t(locale, 'sellerUnknown')}</strong></div>
          <div><span>{locale === 'ar' ? 'وقت القراءة' : 'Reading time'}</span><strong>{dateTime(p.observed_at, locale)}</strong></div>
          <div><span>{locale === 'ar' ? 'المتجر' : 'Store'}</span><strong>{p.store_name}</strong></div>
        </div>
        {p.affiliate_url.startsWith("https://") ?
          <a className="button-primary buy-button" href={p.affiliate_url} target="_blank" rel="noopener noreferrer sponsored">{t(locale, 'openStore')} ↗</a> :
          <div className="link-pending">{t(locale, 'linkPending')}</div>}
        <p className="detail-caution">{t(locale, 'storeCaution')}</p>
      </div>
    </section>
    <section className="history-section">
      <div className="section-heading">
        <div>
          <span className="section-kicker">{t(locale, 'historyKicker')}</span>
          <h2>{t(locale, 'historyTitle')}</h2>
          <p>{t(locale, 'historyIntro')}</p>
        </div>
        <span className="section-mark">/ 02</span>
      </div>
      <div className="history-board">
        <div className="history-stats">
          <div><span>{t(locale, 'minPrice')}</span><strong>{min === null ? "—" : money(String(min), p.currency, locale)}</strong></div>
          <div><span>{t(locale, 'maxPrice')}</span><strong>{max === null ? "—" : money(String(max), p.currency, locale)}</strong></div>
          <div><span>{t(locale, 'directReadings')}</span><strong>{direct.length.toLocaleString(locale === 'ar' ? 'ar-EG' : 'en-US')}</strong></div>
        </div>
        <PriceChart history={detail.history} locale={locale} />
      </div>
      <div className="readings">
        <h3>{t(locale, 'latestReadings')}</h3>
        <div className="reading-list">
          {detail.history.slice(0, 12).map((point, i) => <div key={`${point.at}-${i}`}>
            <span>{dateTime(point.at, locale)}</span>
            <strong>{money(point.price, p.currency, locale)}</strong>
            <small>{point.imported ? t(locale, 'importedLabel') : t(locale, 'directLabel')}</small>
          </div>)}
        </div>
      </div>
    </section>
  </main>;
}
