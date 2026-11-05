import Link from "next/link";
import { dateTime, Deal, money } from "@/lib/api";
import { Locale, t } from "@/lib/i18n";

export default function DealCard({ deal, locale, lead = false }: { deal: Deal; locale: Locale; lead?: boolean }) {
  const initial = deal.store === "amazon_eg" ? "A" : deal.store_name.charAt(0);
  const note = locale === 'ar' ? deal.editorial_note_ar : deal.editorial_note_en;
  return <article className={lead ? "deal-card deal-card-lead" : "deal-card"}>
    <Link href={`/${locale}/products/${deal.id}`} className="deal-image" aria-label={`${t(locale, 'viewDeal')}: ${deal.title}`}>
      {deal.image_url ? <img src={`/api/proxy-image?url=${encodeURIComponent(deal.image_url)}`} alt="" loading={lead ? "eager" : "lazy"} /> : <span className="image-placeholder">{initial}<small>{deal.store_name}</small></span>}
      <span className="deal-verified"><span aria-hidden="true">✓</span> {t(locale, 'verifiedDeal')}</span>
      {deal.displayed_discount ? <span className="deal-discount">{t(locale, 'storeDiscount')} {Number(deal.displayed_discount).toFixed(0)}%</span> : deal.drop_percent ? <span className="deal-discount historical">{t(locale, 'verifiedDrop')} {Number(deal.drop_percent).toFixed(0)}%</span> : null}
    </Link>
    <div className="deal-content">
      <div className="deal-store">{deal.store_name}<span>{deal.country_name}</span></div>
      <Link href={`/${locale}/products/${deal.id}`} className="deal-title">{deal.title}</Link>
      {lead && note && <p className="deal-editorial-note">{note}</p>}
      <div className="deal-price-row">
        <strong>{money(deal.price, deal.currency, locale)}</strong>
        <span>{t(locale, 'lastChecked')}: {dateTime(deal.observed_at, locale)}</span>
      </div>
      <Link href={`/${locale}/products/${deal.id}`} className={lead ? "button-primary deal-action" : "deal-action"}>{t(locale, 'viewDeal')} <span aria-hidden="true">↗</span></Link>
    </div>
  </article>;
}
