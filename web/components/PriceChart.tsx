import { PricePoint } from "@/lib/api";
import { Locale, t } from "@/lib/i18n";

export default function PriceChart({ history, locale }: { history: PricePoint[]; locale: Locale }) {
  const points = [...history].reverse().filter(p => !p.imported).slice(-60);
  if (points.length < 2) return <div className="chart-empty">{locale === 'ar' ? 'محتاجين قراءات مباشرة أكثر عشان نرسم رحلة السعر.' : 'More direct readings are needed to draw the price journey.'}</div>;
  const values = points.map(p => Number(p.price));
  const low = Math.min(...values), high = Math.max(...values), span = Math.max(high - low, 1);
  const coords = values.map((v, i) => `${32 + i * 736 / Math.max(values.length - 1, 1)},${202 - (v - low) / span * 154}`).join(" ");
  const numFmt = locale === 'ar' ? 'ar-EG' : 'en-US';
  const currency = locale === 'ar' ? 'ج' : 'EGP';
  return <div className="chart-wrap" role="img" aria-label={`${locale === 'ar' ? 'رسم سجل السعر من' : 'Price history chart from'} ${low} ${locale === 'ar' ? 'إلى' : 'to'} ${high} ${currency} ${locale === 'ar' ? 'عبر' : 'across'} ${points.length} ${locale === 'ar' ? 'قراءة' : 'readings'}`}>
    <div className="chart-values"><span>{locale === 'ar' ? 'الأعلى' : 'High'}: {high.toLocaleString(numFmt)} {currency}</span><span>{locale === 'ar' ? 'الأقل' : 'Low'}: {low.toLocaleString(numFmt)} {currency}</span></div>
    <svg viewBox="0 0 800 240" preserveAspectRatio="none" aria-hidden="true"><defs><linearGradient id="priceFill" x1="0" y1="0" x2="0" y2="1"><stop stopColor="#1e8e3e" stopOpacity=".22"/><stop offset="1" stopColor="#1e8e3e" stopOpacity="0"/></linearGradient></defs><path d="M 32 48 H 768 M 32 125 H 768 M 32 202 H 768" stroke="#e2ddd3" strokeDasharray="4 7" fill="none"/><polygon points={`32,220 ${coords} 768,220`} fill="url(#priceFill)"/><polyline points={coords} fill="none" stroke="#1e8e3e" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round"/></svg>
    <div className="chart-dates"><span>{new Date(points[0].at).toLocaleDateString(numFmt)}</span><span>{new Date(points.at(-1)!.at).toLocaleDateString(numFmt)}</span></div>
  </div>;
}
