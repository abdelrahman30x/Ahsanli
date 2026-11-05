# Koupnak web

Arabic RTL storefront built with Next.js 16. It serves catalog search, product price history, a verified-deals page, and an explanation page. It reads the Go catalog API over loopback on the server. The public site is `https://134.122.89.95/`; protected operations are at `/dashboard/` and `/admin/`.

```powershell
cd web
npm ci
npm run build
```

The standalone build is deployed under `/root/botAfflite/web_next`, with Node 24.13.0 in `/opt/botafflite-node`. `botafflite-next-preview.service` binds to `127.0.0.1:3001` despite its original staging name. Nginx owns public TLS. `API_INTERNAL_URL=http://127.0.0.1:8082` is set in the unit; no database or Telegram secret is given to Next.

Product pages are server-rendered. Pages with stale or imported prices carry `noindex`; the UI labels those prices as previous readings. Deals require screenshot-matched evidence. The site does not generate fresh prices by itself.
