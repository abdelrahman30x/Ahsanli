"use client";

import { Locale, locales } from "@/lib/i18n";

export default function LanguageSwitcher({ locale }: { locale: Locale }) {
  return (
    <div className="language-switcher">
      <label htmlFor="lang-switch" className="sr-only">Language</label>
      <select
        id="lang-switch"
        value={locale}
        onChange={(e) => {
          const next = e.target.value as Locale;
          const path = window.location.pathname.replace(/^\/(ar|en)/, `/${next}`);
          window.location.href = path + window.location.search;
        }}
      >
        <option value="ar">العربية</option>
        <option value="en">English</option>
      </select>
    </div>
  );
}
