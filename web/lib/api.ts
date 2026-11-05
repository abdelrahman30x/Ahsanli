import { Locale } from "./i18n";

export type Product = {
  id: number; title: string; product_id: string; store: string; store_name: string;
  country: string; country_name: string;
  currency: string; price: string | null; seller: string | null; observed_at: string | null;
  imported: boolean; fresh: boolean; drop_percent: string | null; image_url: string | null;
  url: string; affiliate_url: string;
};
export type PricePoint = { price: string; at: string; imported: boolean; seller: string };
export type ProductPage = { items: Product[]; page?: number; next_cursor?: string; has_more: boolean };
export type ProductDetail = { product: Product; history: PricePoint[] };
export type CategoryMeta = { code: string; name_ar: string; name_en: string };
export type RetailerMeta = { code: string; name: string; enabled: boolean; health: string };
export type CountryMeta = {
  code: string; name_ar: string; name_en: string; currency: string; currency_label: string;
  timezone: string; default_locale: string; hostname: string;
  retailers: RetailerMeta[]; categories: CategoryMeta[]; telegram_url?: string;
};
export type CatalogMeta = { countries: CountryMeta[] };

const api = process.env.API_INTERNAL_URL || "http://127.0.0.1:8083";

export async function getMeta(): Promise<CatalogMeta | null> {
  try {
    const res = await fetch(`${api}/api/v1/meta`, { next: { revalidate: 300 } });
    return res.ok ? await res.json() : null;
  } catch { return null; }
}

export async function getCountryMeta(code: string): Promise<CountryMeta | null> {
  try {
    const res = await fetch(`${api}/api/v1/countries/${code}/meta`, { next: { revalidate: 300 } });
    return res.ok ? await res.json() : null;
  } catch { return null; }
}

export async function listProducts(locale: Locale, country: string, params: Record<string, string>): Promise<ProductPage | null> {
  try {
    const query = new URLSearchParams(params);
    const res = await fetch(`${api}/api/v1/countries/${country}/products?${query}`, { next: { revalidate: 20 } });
    return res.ok ? await res.json() : null;
  } catch { return null; }
}

export async function getProduct(country: string, id: string): Promise<ProductDetail | null> {
  if (!/^\d+$/.test(id)) return null;
  try {
    const res = await fetch(`${api}/api/v1/countries/${country}/products/${id}`, { next: { revalidate: 20 } });
    return res.ok ? await res.json() : null;
  } catch { return null; }
}

export function money(value: string | null, currency = "EGP", locale: Locale = "ar") {
  if (!value) return locale === 'ar' ? 'السعر غير متاح' : 'Price unavailable';
  const fmt = new Intl.NumberFormat(locale === 'ar' ? 'ar-EG' : 'en-US', { maximumFractionDigits: 2 });
  const symbol = currency === 'EGP' ? (locale === 'ar' ? 'ج.م' : 'EGP') : currency;
  return `${fmt.format(Number(value))} ${symbol}`;
}

export function dateTime(value: string | null, locale: Locale = 'ar', timezone = 'Africa/Cairo') {
  if (!value) return locale === 'ar' ? 'لم يُفحص بعد' : 'Not checked yet';
  return new Intl.DateTimeFormat(locale === 'ar' ? 'ar-EG' : 'en-US', { dateStyle: 'medium', timeStyle: 'short', timeZone: timezone }).format(new Date(value));
}
