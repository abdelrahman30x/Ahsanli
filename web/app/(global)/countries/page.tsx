import type { Metadata } from "next";
import { getMeta } from "@/lib/api";
import CountryFlag from "@/components/CountryFlag";
import { SUPPORTED_COUNTRIES } from "@/lib/countries-config";

export const metadata: Metadata = {
  title: "Choose your country | أحسنلي | AHSANLY",
  description: "Select your country to see localized prices and verified deals.",
};

export default async function Countries() {
  const meta = await getMeta();
  const apiCountries = meta?.countries || [];

  // Merge API countries with SUPPORTED_COUNTRIES fallback
  const list = apiCountries.length > 0
    ? apiCountries.map((c) => ({
        code: c.code,
        name_ar: c.name_ar,
        name_en: c.name_en,
        currency: c.currency,
        hostname: c.hostname,
        default_locale: c.default_locale,
      }))
    : SUPPORTED_COUNTRIES.map((c) => ({
        code: c.code,
        name_ar: c.name_ar,
        name_en: c.name_en,
        currency: c.currency,
        hostname: `${c.subdomain}.ahsanli.com`,
        default_locale: "ar",
      }));

  return (
    <main className="country-selector">
      <div className="country-selector-card">
        <h1>اختر بلدك / Choose your country</h1>
        <p>
          اختر بلدك لعرض الأسعار المتاحة لها.
          <br />
          Choose your country to see its available prices.
        </p>
        <div className="country-list">
          {list.map((c) => (
            <a
              key={c.code}
              href={`https://ahsanli.com/api/v1/choose-country?code=${encodeURIComponent(
                c.code
              )}&locale=${encodeURIComponent(c.default_locale || "ar")}`}
              className="country-list-item"
            >
              <span className="country-item-title">
                <CountryFlag code={c.code} width={24} height={16} />
                <span>
                  {c.name_ar} / {c.name_en} ({c.code})
                </span>
              </span>
              <span className="country-item-currency">{c.currency}</span>
            </a>
          ))}
        </div>
        <p className="geo-attribution">
          IP geolocation by{" "}
          <a
            href="https://db-ip.com"
            target="_blank"
            rel="noopener noreferrer"
          >
            DB-IP
          </a>
        </p>
      </div>
    </main>
  );
}
