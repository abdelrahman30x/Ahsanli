# AHSANLY web

Bilingual, mobile-first Next.js storefront for editorially selected store deals. The public home and archive contain only fresh, screenshot-verified offers approved by staff; they do not expose general catalog search or cross-store comparison. Product titles remain retailer-supplied, while navigation, evidence explanations, metadata, and editorial notes support Arabic RTL and English LTR.

```powershell
cd web
npm ci
npm run build
```

The standalone server runs as `ahsanly-web.service` under `/srv/ahsanly/web_next`, bound to `127.0.0.1:3100`. It uses `API_INTERNAL_URL=http://127.0.0.1:8083`; no database or Telegram secret is required by Next.

Routine builds run in GitHub Actions on pushes to `main`. `ahsanly-deploy.timer` on the VPS downloads the tested release, verifies it, and installs it. Manual full-bundle uploads are no longer needed.

Deal detail pages are server-rendered and disappear when their current observation becomes stale or otherwise ineligible. Merchant-displayed discounts and AHSANLY historical drops are separate fields and labels. The site does not generate fresh prices or approve candidates by itself.
