export interface CountryConfig {
  code: string;
  name_ar: string;
  name_en: string;
  subdomain: string;
  currency: string;
  timezone: string;
  default_locale: string;
}

export const SUPPORTED_COUNTRIES: readonly CountryConfig[] = [
  { code: "EG", name_ar: "مصر", name_en: "Egypt", subdomain: "eg", currency: "EGP", timezone: "Africa/Cairo", default_locale: "ar" },
  { code: "SA", name_ar: "السعودية", name_en: "Saudi Arabia", subdomain: "sa", currency: "SAR", timezone: "Asia/Riyadh", default_locale: "ar" },
  { code: "JO", name_ar: "الأردن", name_en: "Jordan", subdomain: "jo", currency: "JOD", timezone: "Asia/Amman", default_locale: "ar" },
  { code: "AE", name_ar: "الإمارات", name_en: "Emirates", subdomain: "ae", currency: "AED", timezone: "Asia/Dubai", default_locale: "ar" },
  { code: "KW", name_ar: "الكويت", name_en: "Kuwait", subdomain: "kw", currency: "KWD", timezone: "Asia/Kuwait", default_locale: "ar" },
  { code: "OM", name_ar: "عُمان", name_en: "Oman", subdomain: "om", currency: "OMR", timezone: "Asia/Muscat", default_locale: "ar" },
  { code: "BH", name_ar: "البحرين", name_en: "Bahrain", subdomain: "bh", currency: "BHD", timezone: "Asia/Bahrain", default_locale: "ar" },
  { code: "QA", name_ar: "قطر", name_en: "Qatar", subdomain: "qa", currency: "QAR", timezone: "Asia/Qatar", default_locale: "ar" },
  { code: "IQ", name_ar: "العراق", name_en: "Iraq", subdomain: "iq", currency: "IQD", timezone: "Asia/Baghdad", default_locale: "ar" },
] as const;

export type SupportedCountryCode = typeof SUPPORTED_COUNTRIES[number]["code"];
