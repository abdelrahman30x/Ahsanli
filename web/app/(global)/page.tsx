// The apex domain is geo-routed by the Go API before reaching Next.js.
// This page is a fallback for direct access to the Next.js origin.
export default function ApexFallback() {
  return <main className="empty-page">
    <span>🌍</span>
    <h1>أحسنلي | AHSANLY</h1>
    <p>Redirecting to country selector…</p>
    <meta httpEquiv="refresh" content="0;url=/countries" />
  </main>;
}
