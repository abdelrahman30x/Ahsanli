import Link from "next/link";
import { dateTime, money, Product } from "@/lib/api";
import { Locale, t } from "@/lib/i18n";

export default function ProductCard({ product, locale }: { product: Product; locale: Locale }) {
  const initial = product.store === "amazon_eg" ? "A" : product.store === "noon_eg" ? "N" : "J";
  const statusClass = product.fresh ? "status fresh" : product.imported ? "status imported" : "status stale";
  const statusText = product.fresh ? t(locale, 'verifiedPrice') : product.imported ? t(locale, 'importedPrice') : t(locale, 'stalePrice');
  return <article className="product-card">
    <Link href={`/${locale}/products/${product.id}`} className="product-image" aria-label={`${locale === 'ar' ? 'تفاصيل' : 'Details of'} ${product.title}`}>
      {product.image_url ? <img src={product.image_url} alt="" loading="lazy" referrerPolicy="no-referrer" /> : <span className="image-placeholder">{initial}<small>{locale === 'ar' ? 'الصورة غير متاحة' : 'No image'}</small></span>}
      {product.drop_percent && product.fresh && <span className="discount-chip">↓ {Number(product.drop_percent).toFixed(1)}%</span>}
    </Link>
    <div className="product-content">
      <div className="product-topline"><span>{product.store_name}</span><span className={statusClass}>{statusText}</span></div>
      <Link href={`/${locale}/products/${product.id}`} className="product-title">{product.title}</Link>
      <div className="product-price">{money(product.price, product.currency, locale)}</div>
      <div className="product-meta">
        <span>{!product.imported && product.seller ? `${locale === 'ar' ? 'البائع' : 'Seller'}: ${product.seller}` : t(locale, 'sellerUnknown')}</span>
        <span>{dateTime(product.observed_at, locale)}</span>
      </div>
      <Link href={`/${locale}/products/${product.id}`} className="card-action">{locale === 'ar' ? 'شوف سجل السعر' : 'View price history'} <span aria-hidden="true">↗</span></Link>
    </div>
  </article>;
}
