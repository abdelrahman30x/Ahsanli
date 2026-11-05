import { headers, cookies } from "next/headers";
import { getCountryMeta, CountryMeta } from "./api";
import { SUPPORTED_COUNTRIES, CountryConfig } from "./countries-config";

export { SUPPORTED_COUNTRIES };
export type { CountryConfig };

export interface ResolvedCountry {
  code: string;
  meta: CountryMeta | null;
  hostname: string;
  name_ar: string;
  name_en: string;
}

export async function resolveCurrentCountry(): Promise<ResolvedCountry> {
  let host = "";
  try {
    const headerList = await headers();
    host = headerList.get("x-forwarded-host") || headerList.get("host") || "";
  } catch {
    // static generation or build phase
  }

  const hostNoPort = host.split(":")[0].toLowerCase();
  const sub = hostNoPort.split(".")[0]?.toUpperCase() || "";

  // Check if sub matches any supported country
  const matchedConfig = SUPPORTED_COUNTRIES.find(
    (c) => c.code === sub || c.subdomain === sub.toLowerCase()
  );

  let targetCode = "EG";
  if (matchedConfig) {
    targetCode = matchedConfig.code;
  } else {
    // Apex domain or localhost - check cookie preference if present
    try {
      const cookieJar = await cookies();
      const pref = cookieJar.get("country_pref")?.value?.toUpperCase();
      if (pref && SUPPORTED_COUNTRIES.some((c) => c.code === pref)) {
        targetCode = pref;
      }
    } catch {
      // ignore
    }
  }

  const meta = await getCountryMeta(targetCode);
  const fallback =
    SUPPORTED_COUNTRIES.find((c) => c.code === targetCode) ||
    SUPPORTED_COUNTRIES[0];
  const hostname = meta?.hostname || `${fallback.subdomain}.ahsanli.com`;

  return {
    code: targetCode,
    meta,
    hostname,
    name_ar: meta?.name_ar || fallback.name_ar,
    name_en: meta?.name_en || fallback.name_en,
  };
}
