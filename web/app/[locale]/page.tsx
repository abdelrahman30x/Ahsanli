import Link from "next/link";
import ProductCard from "@/components/ProductCard";
import { listProducts, getCountryMeta } from "@/lib/api";
import { Locale, t, direction } from "@/lib/i18n";

type Params = { locale: Locale };
type SearchParams = Record<string, string | string[] | undefined>;
function one(value: string | string[] | undefined) { return typeof value === "string" ? value : ""; }

export default async function Home({ params, searchParams }: { params: Promise<Params>; searchParams: Promise<SearchParams> }) {
  const { locale } = await params;
  const raw = await searchParams;
  const q = one(raw.q), store = one(raw.store), category = one(raw.category), sort = one(raw.sort) || "recent", verified = one(raw.verified);
  const cursor = one(raw.cursor);
  const minPrice = one(raw.min_price), maxPrice = one(raw.max_price);
  const country = 'EG';
  const countryMeta = await getCountryMeta(country);
  const paramsObj: Record<string, string> = { sort, limit: "24" };
  if (q) paramsObj.q = q;
  if (store) paramsObj.store = store;
  if (category) paramsObj.category = category;
  if (verified) paramsObj.verified = verified;
  if (cursor) paramsObj.cursor = cursor;
  if (minPrice) paramsObj.min_price = minPrice;
  if (maxPrice) paramsObj.max_price = maxPrice;
  const products = await listProducts(locale, country, paramsObj);
  const stores = countryMeta?.retailers || [];
  const categories = countryMeta?.categories || [];
  const dir = direction(locale);

  const nextParams = new URLSearchParams({ q, store, category, sort, verified, min_price: minPrice, max_price: maxPrice });
  if (products?.next_cursor) nextParams.set('cursor', products.next_cursor);

  return <main>
    <section className="hero">
      <div className="hero-copy">
        <span className="eyebrow"><span className="signal-dot"/> {t(locale, 'heroEyebrow')}</span>
        <h1>{t(locale, 'heroTitle').split('\n').map((line, i) => <span key={i}>{line}<br/></span>)}</h1>
        <p>{t(locale, 'heroBody')}</p>
        <div className="hero-links">
          <a href="#catalog" className="button-primary">{t(locale, 'heroCta')} <span>{dir === 'rtl' ? '↙' : '↘'}</span></a>
          <a href="#how" className="button-text">{t(locale, 'heroHow')}</a>
        </div>
      </div>
      <div className="hero-visual" aria-hidden="true">
        <div className="hero-ticket">
          <div>{locale === 'ar' ? 'مؤشر السعر' : 'Price index'}</div>
          <strong>EGP</strong>
          <span>{locale === 'ar' ? 'سعر موثق عند توافره' : 'Verified when available'}</span>
        </div>
        <div className="hero-note">{locale === 'ar' ? 'السعر × البائع × وقت الفحص' : 'Price × Seller × Check time'}</div>
      </div>
    </section>

    <section className="trust-strip">
      <span><b>01</b> {t(locale, 'trustSource')}</span>
      <span><b>02</b> {t(locale, 'trustHistory')}</span>
      <span><b>03</b> {t(locale, 'trustStatus')}</span>
      <span><b>04</b> {t(locale, 'trustLink')}</span>
    </section>

    <section id="catalog" className="catalog-section">
      <div className="section-heading">
        <div>
          <span className="section-kicker">{t(locale, 'catalogKicker')}</span>
          <h2>{t(locale, 'catalogTitle')}</h2>
          <p>{t(locale, 'catalogIntro')}</p>
        </div>
        <span className="section-mark">/ 01</span>
      </div>
      <form className="filter-panel" action={`/${locale}/`} method="get">
        <label className="search-field">
          <span>{locale === 'ar' ? 'بحث عن منتج' : 'Search products'}</span>
          <input name="q" defaultValue={q} placeholder={t(locale, 'searchPlaceholder')} />
        </label>
        <label>
          <span>{t(locale, 'filterStore')}</span>
          <select name="store" defaultValue={store}>
            <option value="">{t(locale, 'allStores')}</option>
            {stores.map(s => <option key={s.code} value={s.code}>{s.name}</option>)}
          </select>
        </label>
        <label>
          <span>{t(locale, 'filterCategory')}</span>
          <select name="category" defaultValue={category}>
            <option value="">{t(locale, 'allCategories')}</option>
            {categories.map(c => <option key={c.code} value={c.code}>{locale === 'ar' ? c.name_ar : c.name_en}</option>)}
          </select>
        </label>
        <label>
          <span>{t(locale, 'filterSort')}</span>
          <select name="sort" defaultValue={sort}>
            <option value="recent">{t(locale, 'sortRecent')}</option>
            <option value="drop">{t(locale, 'sortDrop')}</option>
            <option value="price_asc">{t(locale, 'sortPriceAsc')}</option>
            <option value="price_desc">{t(locale, 'sortPriceDesc')}</option>
          </select>
        </label>
        <label>
          <span>{t(locale, 'filterVerified')}</span>
          <select name="verified" defaultValue={verified}>
            <option value="">{t(locale, 'allReadings')}</option>
            <option value="1">{t(locale, 'verifiedOnly')}</option>
          </select>
        </label>
        <label><span>{locale === 'ar' ? 'من سعر' : 'Min price'}</span><input name="min_price" type="number" min="0" step="0.01" defaultValue={minPrice} /></label>
        <label><span>{locale === 'ar' ? 'إلى سعر' : 'Max price'}</span><input name="max_price" type="number" min="0" step="0.01" defaultValue={maxPrice} /></label>
        <button className="button-primary" type="submit">{t(locale, 'showResults')} {dir === 'rtl' ? '←' : '→'}</button>
      </form>
      {!products ? <div className="empty-state"><strong>{t(locale, 'catalogUnavailable')}</strong><p>{t(locale, 'emptyResultsHint')}</p></div> :
       products.items.length === 0 ? <div className="empty-state"><strong>{t(locale, 'emptyResults')}</strong><p>{t(locale, 'emptyResultsHint')}</p></div> :
       <>
        <div className="results-line">
          <span>{t(locale, 'page')} {products.page || 1} · {products.items.length} {t(locale, 'product')}</span>
          <span>{locale === 'ar' ? 'اضغط على المنتج لعرض سجله الكامل' : 'Click a product to view its full history'}</span>
        </div>
        <div className="product-grid">{products.items.map(p => <ProductCard key={p.id} product={p} locale={locale} />)}</div>
        <div className="pagination">
          {products.has_more && <Link href={`/${locale}/?${nextParams}`}>{t(locale, 'next')} {dir === 'rtl' ? '←' : '→'}</Link>}
        </div>
       </>}
    </section>

    <section id="how" className="how-section">
      <div className="section-heading">
        <div>
          <span className="section-kicker">{t(locale, 'howKicker')}</span>
          <h2>{t(locale, 'howTitle')}</h2>
        </div>
      </div>
      <div className="how-grid">
        <div><span>01 / {t(locale, 'howObserveTitle')}</span><h3>{t(locale, 'howObserveTitle')}</h3><p>{t(locale, 'howObserveBody')}</p></div>
        <div><span>02 / {t(locale, 'howCompareTitle')}</span><h3>{t(locale, 'howCompareTitle')}</h3><p>{t(locale, 'howCompareBody')}</p></div>
        <div><span>03 / {t(locale, 'howDecideTitle')}</span><h3>{t(locale, 'howDecideTitle')}</h3><p>{t(locale, 'howDecideBody')}</p></div>
      </div>
    </section>
  </main>;
}
