"use client";

import { useState, useRef, useEffect } from "react";
import { Locale, t } from "@/lib/i18n";
import CountryFlag from "./CountryFlag";
import { SUPPORTED_COUNTRIES } from "@/lib/countries-config";

interface CountrySwitcherProps {
  currentCountry: string; // e.g. 'EG', 'SA'
  locale: Locale;
}

export default function CountrySwitcher({
  currentCountry,
  locale,
}: CountrySwitcherProps) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  const activeCountry =
    SUPPORTED_COUNTRIES.find(
      (c) => c.code === currentCountry.toUpperCase()
    ) || SUPPORTED_COUNTRIES[0];

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (
        containerRef.current &&
        !containerRef.current.contains(event.target as Node)
      ) {
        setOpen(false);
      }
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
      }
    }

    document.addEventListener("mousedown", handleClickOutside);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handleClickOutside);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, []);

  const handleSelectCountry = (code: string) => {
    // Set preference cookie for .ahsanli.com
    try {
      const isAhsanli = window.location.hostname.includes("ahsanli.com");
      const domainAttr = isAhsanli ? "; domain=.ahsanli.com" : "";
      document.cookie = `country_pref=${code}; path=/${domainAttr}; max-age=31536000; SameSite=Lax; Secure`;
    } catch {
      // ignore
    }
  };

  const getCountryHref = (subdomain: string, code: string) => {
    // If running in development / localhost, retain port
    if (typeof window !== "undefined") {
      const host = window.location.hostname;
      if (host.includes("localhost") || host.includes("127.0.0.1")) {
        return `http://${subdomain}.${host}:${window.location.port}/${locale}/`;
      }
    }
    return `https://${subdomain}.ahsanli.com/${locale}/`;
  };

  return (
    <div
      ref={containerRef}
      className={`country-switcher-wrap w3-dropdown-hover country-w3-dropdown-hover ${
        open ? "is-open" : ""
      }`}
    >
      <button
        type="button"
        className="w3-button active-w3-button country-switcher-trigger"
        aria-label={t(locale, "selectCountry")}
        aria-expanded={open}
        aria-haspopup="true"
        onClick={() => setOpen((prev) => !prev)}
      >
        <i
          className={`country-icon icon-${activeCountry.subdomain}`}
          aria-hidden="true"
        >
          <CountryFlag code={activeCountry.code} width={20} height={14} />
        </i>
        <span className="country-current-code">{activeCountry.code}</span>
        <span className="country-caret" aria-hidden="true">
          ▾
        </span>
      </button>

      <div
        className={`w3-dropdown-content w3-bar-block w3-border country-dropdown-list ${
          open ? "w3-show" : ""
        }`}
        role="menu"
        aria-label={t(locale, "selectCountry")}
      >
        {SUPPORTED_COUNTRIES.map((c) => {
          const isCurrent = c.code === activeCountry.code;
          const countryLabel = locale === "ar" ? c.name_ar : c.name_en;
          const title = `${c.name_en} / ${c.name_ar}`;

          return (
            <a
              key={c.code}
              title={title}
              href={getCountryHref(c.subdomain, c.code)}
              className={`w3-button country-item ${isCurrent ? "active-country" : ""}`}
              role="menuitem"
              onClick={() => handleSelectCountry(c.code)}
            >
              <i
                className={`country-icon icon-${c.subdomain}`}
                aria-hidden="true"
              >
                <CountryFlag code={c.code} width={20} height={14} />
              </i>
              <span className="country-label">{countryLabel}</span>
              <span className="country-currency-tag">{c.currency}</span>
              {isCurrent && (
                <span className="country-active-check" aria-hidden="true">
                  ✓
                </span>
              )}
            </a>
          );
        })}
      </div>
    </div>
  );
}
