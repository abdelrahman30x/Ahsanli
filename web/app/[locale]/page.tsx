import Link from "next/link";
import DealCard from "@/components/DealCard";
import { listDeals, getCountryMeta } from "@/lib/api";
import { Locale, t, direction } from "@/lib/i18n";

type Params = { locale: Locale };

export default async function Home({ params }: { params: Promise<Params> }) {
  const { locale } = await params;
  const country = 'EG';
  const [countryMeta, deals] = await Promise.all([
    getCountryMeta(country),
    listDeals(country, { limit: "13" }),
  ]);
  const stores = (countryMeta?.retailers || []).filter(store => store.enabled);
  const categories = countryMeta?.categories || [];
  const dir = direction(locale);
  const items = deals?.items || [];
  const featured = items.find(deal => deal.featured);
  const latest = featured ? items.filter(deal => deal.deal_id !== featured.deal_id) : items;

  return <main>
    <section className="deals-hero">
      <div className="hero-copy">
        <span className="eyebrow"><span className="signal-dot"/> {t(locale, 'heroEyebrow')}</span>
        <h1>{t(locale, 'heroTitle').split('\n').map((line, i) => <span key={i}>{line}<br/></span>)}</h1>
        <p>{t(locale, 'heroBody')}</p>
        <div className="hero-links">
          <Link href="#deals" className="button-primary">{t(locale, 'heroCta')} <span>{dir === 'rtl' ? '↙' : '↘'}</span></Link>
          <Link href="#how" className="button-text">{t(locale, 'heroHow')}</Link>
        </div>
        {countryMeta?.telegram_url && <div className="hero-telegram"><a href={countryMeta.telegram_url} target="_blank" rel="noopener noreferrer">{t(locale, 'telegramCta')} ↗</a><span>{t(locale, 'telegramHint')}</span></div>}
      </div>
      <div className="deals-hero-visual" aria-hidden="true">
        <div className="selection-stamp"><span>أحسنلي</span><strong>✓</strong><small>{locale === 'ar' ? 'اختيار بعد مراجعة' : 'Selected after review'}</small></div>
        <div className="hero-price-tag"><span>{locale === 'ar' ? 'خصم حقيقي' : 'Real discount'}</span><strong>%</strong><small>{locale === 'ar' ? 'من المتجر · بدليل' : 'From the store · With evidence'}</small></div>
      </div>
    </section>

    <section className="trust-strip">
      <span><b>01</b> {t(locale, 'trustSource')}</span>
      <span><b>02</b> {t(locale, 'trustStatus')}</span>
      <span><b>03</b> {t(locale, 'trustHistory')}</span>
    </section>

    {featured && <section className="featured-section">
      <div className="section-heading"><div><span className="section-kicker">{t(locale, 'featuredKicker')}</span><h2>{t(locale, 'featuredTitle')}</h2></div><span className="section-mark">★</span></div>
      <DealCard deal={featured} locale={locale} lead />
    </section>}

    <section id="deals" className="deals-section">
      <div className="section-heading">
        <div><span className="section-kicker">{t(locale, 'latestKicker')}</span><h2>{t(locale, 'latestTitle')}</h2><p>{t(locale, 'latestIntro')}</p></div>
        <Link href={`/${locale}/deals`} className="section-link">{t(locale, 'viewAllDeals')} {dir === 'rtl' ? '←' : '→'}</Link>
      </div>
      {!deals ? <div className="empty-state"><strong>{t(locale, 'catalogUnavailable')}</strong><p>{t(locale, 'emptyResultsHint')}</p></div> : latest.length === 0 ? <div className="empty-state deals-empty"><span className="empty-tag" aria-hidden="true">%</span><strong>{t(locale, 'noDeals')}</strong><p>{t(locale, 'noDealsHint')}</p>{countryMeta?.telegram_url && <a className="button-secondary" href={countryMeta.telegram_url} target="_blank" rel="noopener noreferrer">{t(locale, 'navTelegram')} ↗</a>}</div> : <div className="deal-grid">{latest.slice(0, 12).map(deal => <DealCard key={deal.deal_id} deal={deal} locale={locale} />)}</div>}
    </section>

    {(stores.length > 0 || categories.length > 0) && <section id="stores" className="discovery-section">
      {stores.length > 0 && <div className="discovery-group"><div className="section-heading"><div><span className="section-kicker">{t(locale, 'storesKicker')}</span><h2>{t(locale, 'storesTitle')}</h2></div></div><div className="discovery-links">{stores.map(store => <Link key={store.code} href={`/${locale}/deals?store=${store.code}`}><span className="store-initial">{store.name.charAt(0)}</span><strong>{store.name}</strong><span aria-hidden="true">↗</span></Link>)}</div></div>}
      {categories.length > 0 && <div className="discovery-group"><div className="section-heading"><div><span className="section-kicker">{t(locale, 'categoriesKicker')}</span><h2>{t(locale, 'categoriesTitle')}</h2></div></div><div className="category-links">{categories.map(category => <Link key={category.code} href={`/${locale}/deals?category=${category.code}`}>{locale === 'ar' ? category.name_ar : category.name_en}<span aria-hidden="true">↗</span></Link>)}</div></div>}
    </section>}

    <section id="how" className="how-section">
      <div className="section-heading"><div><span className="section-kicker">{t(locale, 'howKicker')}</span><h2>{t(locale, 'howTitle')}</h2></div></div>
      <div className="how-grid">
        <div><span>01</span><h3>{t(locale, 'howObserveTitle')}</h3><p>{t(locale, 'howObserveBody')}</p></div>
        <div><span>02</span><h3>{t(locale, 'howCompareTitle')}</h3><p>{t(locale, 'howCompareBody')}</p></div>
        <div><span>03</span><h3>{t(locale, 'howDecideTitle')}</h3><p>{t(locale, 'howDecideBody')}</p></div>
      </div>
    </section>
  </main>;
}
