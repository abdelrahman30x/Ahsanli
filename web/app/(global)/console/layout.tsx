import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "إدارة أحسنلي | AHSANLY Console",
  robots: { index: false, follow: false },
};

export default function ConsoleLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return children;
}
