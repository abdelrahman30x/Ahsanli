import type { Metadata } from "next";
import { getMeta } from "@/lib/api";

export const metadata: Metadata = {
  title: "Choose your country | أحسنلي | AHSANLY",
  description: "Select your country to see localized prices and verified deals.",
};

export default async function Countries() {
  const meta = await getMeta();
  const countries = meta?.countries || [];
  return <main className="country-selector">
    <div className="country-selector-card">
      <h1>اختر بلدك / Choose your country</h1>
      <p>اختر بلدك لعرض الأسعار المتاحة لها.<br/>Choose your country to see its available prices.</p>
      <div className="country-list">
        {countries.map(c => (
          <a key={c.code} href={`https://ahsanli.com/api/v1/choose-country?code=${encodeURIComponent(c.code)}&locale=${encodeURIComponent(c.default_locale)}`}>
            <span>{c.name_ar} / {c.name_en} ({c.code})</span>
            <span>{c.currency}</span>
          </a>
        ))}
        {countries.length === 0 && <p>قائمة الدول غير متاحة حاليًا. حاول مرة أخرى بعد قليل.<br/>The country list is temporarily unavailable.</p>}
      </div>
      <p className="geo-attribution">IP geolocation by <a href="https://db-ip.com" target="_blank" rel="noopener noreferrer">DB-IP</a></p>
    </div>
  </main>;
}
