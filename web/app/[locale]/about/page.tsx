import type { Metadata } from "next";
import { Locale, t } from "@/lib/i18n";

export async function generateMetadata({ params }: { params: Promise<{ locale: Locale }> }): Promise<Metadata> {
  const { locale } = await params;
  return {
    title: `${t(locale, 'navAbout')} | ${t(locale, 'brand')}`,
    description: t(locale, 'aboutIntro'),
  };
}

export default async function About({ params }: { params: Promise<{ locale: Locale }> }) {
  const { locale } = await params;
  return <main className="catalog-section standalone-page">
    <div className="section-heading">
      <div>
        <span className="section-kicker">{t(locale, 'navAbout')}</span>
        <h1>{t(locale, 'aboutTitle')}</h1>
        <p>{t(locale, 'aboutIntro')}</p>
      </div>
      <span className="section-mark">/ ABOUT</span>
    </div>
    <div className="how-grid">
      <div><span>01 / {t(locale, 'aboutDirectTitle')}</span><h2>{t(locale, 'aboutDirectTitle')}</h2><p>{t(locale, 'aboutDirectBody')}</p></div>
      <div><span>02 / {t(locale, 'aboutSameTitle')}</span><h2>{t(locale, 'aboutSameTitle')}</h2><p>{t(locale, 'aboutSameBody')}</p></div>
      <div><span>03 / {t(locale, 'aboutEvidenceTitle')}</span><h2>{t(locale, 'aboutEvidenceTitle')}</h2><p>{t(locale, 'aboutEvidenceBody')}</p></div>
    </div>
    <div className="about-note">
      <h2>{t(locale, 'aboutShippingTitle')}</h2>
      <p>{t(locale, 'aboutShippingBody')}</p>
    </div>
  </main>;
}
