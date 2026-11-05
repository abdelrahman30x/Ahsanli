import type { Metadata } from "next";
import "../globals.css";

export const metadata: Metadata = {
  title: "أحسنلي | AHSANLY",
  description: "أحسن عرض.. أحسن سعر — أسعار موثقة وعروض حقيقية حسب بلدك.",
};

export default function GlobalLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="ar" dir="rtl"><body>{children}</body></html>;
}
