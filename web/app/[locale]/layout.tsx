import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { locales, direction, t, Locale } from "@/lib/i18n";
import LanguageSwitcher from "@/components/LanguageSwitcher";
import CountrySwitcher from "@/components/CountrySwitcher";
import MobileMenu from "@/components/MobileMenu";
import { resolveCurrentCountry } from "@/lib/country";
import "../globals.css";

export async function generateStaticParams() {
  return locales.map((locale) => ({ locale }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  const lang = locales.includes(locale as Locale) ? (locale as Locale) : "ar";
  const resolved = await resolveCurrentCountry();

  return {
    title: `${t(lang, "brand")} | ${t(lang, "tagline")}`,
    description: t(lang, "heroBody"),
    alternates: {
      canonical:
        lang === "ar"
          ? `https://${resolved.hostname}/ar/`
          : `https://${resolved.hostname}/en/`,
      languages: {
        [`ar-${resolved.code}`]: `https://${resolved.hostname}/ar/`,
        en: `https://${resolved.hostname}/en/`,
      },
    },
  };
}

export default async function LocaleLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  if (!locales.includes(locale as Locale)) notFound();
  const lang = locale as Locale;
  const dir = direction(lang);
  const resolved = await resolveCurrentCountry();
  const country = resolved.meta;
  const countryName = lang === "ar" ? resolved.name_ar : resolved.name_en;

  return (
    <html lang={lang} dir={dir}>
      <head>
        <link rel="icon" href="/logo-mark.svg" type="image/svg+xml" />
      </head>
      <body>
        <div className="site-shell">
          <header className="site-header">
            <Link
              href={`/${lang}/`}
              className="brand"
              aria-label={t(lang, "brand")}
            >
              <span className="brand-mark" aria-hidden="true">
                <img src="/logo-mark.svg" alt="" width="38" height="38" />
              </span>
              <span>{t(lang, "brand")}</span>
              <span className="brand-tagline">{t(lang, "tagline")}</span>
            </Link>
            <nav className="desktop-nav" aria-label={t(lang, "mainNavigation")}>
              <Link href={`/${lang}/deals`}>{t(lang, "navDeals")}</Link>
              <Link href={`/${lang}/#stores`}>{t(lang, "navStores")}</Link>
              <Link href={`/${lang}/#how`}>{t(lang, "navHow")}</Link>
              {country?.telegram_url && (
                <a
                  className="nav-telegram"
                  href={country.telegram_url}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {t(lang, "navTelegram")} ↗
                </a>
              )}
            </nav>
            <div className="header-actions">
              <CountrySwitcher currentCountry={resolved.code} locale={lang} />
              <LanguageSwitcher locale={lang} />
              <MobileMenu
                locale={lang}
                countryName={countryName}
                currentCountryCode={resolved.code}
                telegramUrl={country?.telegram_url}
              />
            </div>
          </header>
          {children}
          <footer>
            <div>
              <strong>{t(lang, "brand")}</strong>
              <p>{t(lang, "footerClaim")}</p>
            </div>
            <div>
              {t(lang, "footerDisclaimer")}
              <br />
              <Link href="/countries">
                {lang === "ar" ? "تغيير البلد" : "Change country"}: {countryName}
              </Link>
              <br />
              <a
                href="https://db-ip.com"
                target="_blank"
                rel="noopener noreferrer"
              >
                IP geolocation by DB-IP
              </a>
            </div>
          </footer>
        </div>
      </body>
    </html>
  );
}
