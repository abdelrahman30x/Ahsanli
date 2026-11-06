"use client";

import { useState } from "react";
import Link from "next/link";
import { Locale, direction, t } from "@/lib/i18n";
import CountryFlag from "./CountryFlag";
import { SUPPORTED_COUNTRIES } from "@/lib/countries-config";

export default function MobileMenu({
  locale,
  countryName,
  currentCountryCode = "EG",
  telegramUrl,
}: {
  locale: Locale;
  countryName: string;
  currentCountryCode?: string;
  telegramUrl?: string;
}) {
  const [open, setOpen] = useState(false);
  const [showCountries, setShowCountries] = useState(false);
  const dir = direction(locale);
  const close = () => {
    setOpen(false);
    setShowCountries(false);
  };

  const getCountryHref = (subdomain: string) => {
    if (typeof window !== "undefined") {
      const host = window.location.hostname;
      if (host.includes("localhost") || host.includes("127.0.0.1")) {
        return `http://${subdomain}.${host}:${window.location.port}/${locale}/`;
      }
    }
    return `https://${subdomain}.ahsanli.com/${locale}/`;
  };

  const handleSelectCountry = (code: string) => {
    try {
      const isAhsanli = window.location.hostname.includes("ahsanli.com");
      const domainAttr = isAhsanli ? "; domain=.ahsanli.com" : "";
      document.cookie = `country_pref=${code}; path=/${domainAttr}; max-age=31536000; SameSite=Lax; Secure`;
    } catch {
      // ignore
    }
    close();
  };

  return (
    <>
      <button
        type="button"
        className="mobile-menu-button"
        aria-label={t(locale, "menu")}
        aria-expanded={open}
        aria-controls="mobile-menu"
        onClick={() => setOpen((v) => !v)}
      >
        <span aria-hidden="true" />
        <span aria-hidden="true" />
        <span aria-hidden="true" />
      </button>

      {open && (
        <div
          className="mobile-menu-backdrop"
          aria-hidden="true"
          onClick={close}
        />
      )}

      <div
        id="mobile-menu"
        className={`mobile-menu ${open ? "open" : ""}`}
        dir={dir}
        aria-hidden={!open}
      >
        <div className="mobile-menu-header">
          <Link href={`/${locale}/`} className="brand" onClick={close}>
            <span className="brand-mark" aria-hidden="true">
              <img src="/logo-mark.svg" alt="" width="34" height="34" />
            </span>
            <span>{t(locale, "brand")}</span>
          </Link>
          <button
            type="button"
            className="mobile-menu-close"
            aria-label={t(locale, "close")}
            onClick={close}
          >
            ×
          </button>
        </div>

        <nav aria-label={t(locale, "mainNavigation")}>
          <Link href={`/${locale}/deals`} onClick={close}>
            {t(locale, "navDeals")}
          </Link>
          <Link href={`/${locale}/#stores`} onClick={close}>
            {t(locale, "navStores")}
          </Link>
          <Link href={`/${locale}/#how`} onClick={close}>
            {t(locale, "navHow")}
          </Link>
          <Link href={`/${locale}/about`} onClick={close}>
            {t(locale, "navAbout")}
          </Link>
          {telegramUrl && (
            <a
              className="mobile-telegram-link"
              href={telegramUrl}
              target="_blank"
              rel="noopener noreferrer"
              onClick={close}
            >
              {t(locale, "navTelegram")} ↗
            </a>
          )}
        </nav>

        <div className="mobile-menu-footer">
          <button
            type="button"
            className="country-row"
            onClick={() => setShowCountries(!showCountries)}
            aria-expanded={showCountries}
          >
            <span style={{ display: "inline-flex", alignItems: "center", gap: "8px" }}>
              <CountryFlag code={currentCountryCode} width={20} height={14} />
              <span>{t(locale, "changeCountry")}</span>
            </span>
            <strong style={{ display: "inline-flex", alignItems: "center", gap: "4px" }}>
              {countryName}
              <span style={{ fontSize: "12px", opacity: 0.7 }}>{showCountries ? "▴" : "▾"}</span>
            </strong>
          </button>

          {showCountries && (
            <div className="mobile-country-list">
              {SUPPORTED_COUNTRIES.map((c) => {
                const isCurrent = c.code === currentCountryCode.toUpperCase();
                const label = locale === "ar" ? c.name_ar : c.name_en;
                return (
                  <a
                    key={c.code}
                    href={getCountryHref(c.subdomain)}
                    className={`mobile-country-item ${isCurrent ? "active" : ""}`}
                    onClick={() => handleSelectCountry(c.code)}
                  >
                    <CountryFlag code={c.code} width={18} height={12} />
                    <span>{label}</span>
                    <small>{c.currency}</small>
                    {isCurrent && <span style={{ marginInlineStart: "auto" }}>✓</span>}
                  </a>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </>
  );
}
