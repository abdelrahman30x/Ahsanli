"use client";

import { useState } from "react";
import Link from "next/link";
import { Locale, direction, t } from "@/lib/i18n";

export default function MobileMenu({
  locale,
  countryName,
  telegramUrl,
}: {
  locale: Locale;
  countryName: string;
  telegramUrl?: string;
}) {
  const [open, setOpen] = useState(false);
  const dir = direction(locale);
  const close = () => setOpen(false);

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
              <img src="/logo-mark.svg" alt="" width="24" height="24" />
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
          <Link href={`/${locale}/`} onClick={close}>
            {t(locale, "navProducts")}
          </Link>
          <Link href={`/${locale}/deals`} onClick={close}>
            {t(locale, "navDeals")}
          </Link>
          <Link href={`/${locale}/about`} onClick={close}>
            {t(locale, "navAbout")}
          </Link>
          {telegramUrl && (
            <a
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
          <Link href="/countries" className="country-row" onClick={close}>
            <span>{t(locale, "changeCountry")}</span>
            <strong>{countryName}</strong>
          </Link>
        </div>
      </div>
    </>
  );
}
