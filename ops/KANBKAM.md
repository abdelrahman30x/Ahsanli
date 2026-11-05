# Kanbkam Egypt collection

Kanbkam is a **discovery and price-hint source**, not a verified retailer offer. Its prices never enter `OfferObservation` or trigger Telegram. A deal still requires a fresh direct price, seller, and screenshot from the underlying store.

**2026-11-02 status:** Both Windows tasks below were disabled at the user's request, and the running collector was stopped. The following table documents their prior behavior. No Kanbkam collection or sync runs on the PC now.

## Running components

| Component | Location | Behavior |
| --- | --- | --- |
| Collector | Windows task `BotAfflite Kanbkam Collector` | `pythonw.exe kanbkam_collect.py run --max-requests 1000 --details-per-page 8 --delay 1.5`, every 30 minutes, one instance. |
| Local checkpoint | `data/kanbkam/kanbkam.sqlite3` | Category cursors, unique products, brands, source marketplace and ID, price hints, due detail checks, and hint-price changes. SQLite WAL is enabled. |
| Sync | Windows task `BotAfflite Kanbkam Sync` | Every hour, uses SQLite online backup, SCP, and `manage.py import_kanbkam --apply`. |
| PostgreSQL | `core.KanbkamCategory`, `KanbkamProduct`, `KanbkamProductCategory`, `KanbkamPriceChange` | Read-only in Django admin. Separate from retailer offer observations and publications. |
| Server snapshot | `/root/botAfflite/data/kanbkam/import.sqlite3` | Last transferred consistent SQLite backup. |

The server IP returned HTTP 403 for Kanbkam on 2026-10-31. Normal HTTP requests from the Windows PC worked without browser or supplied cookies, so collection runs there. The home menu provided 14 parent groups and 116 leaf categories. The four additional `/l` links outside the menu were promotional pages, not product subcategories.

## Collection rules

- Read the homepage category menu each run; add new categories without resetting existing cursors.
- Request `/eg/ar/getFilterData` with `slug`, `page`, `inputs`, and `skip`. Advance `skip` by `count` after each verified page. A successful response with `count=0` and empty `html` completes that category. Repeated pages or malformed cards stop with an error and keep the last checkpoint.
- Deduplicate products by `(marketplace, product_id)`, using the card's `data-gtmid` source and URL ID. Keep every product-to-category membership.
- Visit category pages in round-robin order so a very large category does not prevent the other categories from starting. Check up to eight due product pages after each category page.
- Fetch product pages to read JSON-LD brand, title, visible price hint, and availability hint. Failed product pages retry later without blocking other products. Full category scans repeat after seven days; product detail checks become due after 24 hours, subject to queue capacity.
- Do not replay the cookies or XSRF tokens pasted in chat. Do not fetch `/autoUpdateProduct`, which the site's robots rules disallow.

## Commands

```powershell
python kanbkam_collect.py summary
python kanbkam_collect.py run --category mobile-phone --max-requests 100 --delay 1.5
python kanbkam_sync.py
Get-ScheduledTask -TaskName 'BotAfflite Kanbkam Collector','BotAfflite Kanbkam Sync'
Get-Content data\kanbkam\collector.log -Tail 30
Get-Content data\kanbkam\sync.log -Tail 30
```

On the VPS, with `/root/botAfflite/deal.env` loaded:

```bash
cd /root/botAfflite
deal_venv/bin/python manage.py import_kanbkam --sqlite data/kanbkam/import.sqlite3
deal_venv/bin/python manage.py import_kanbkam --sqlite data/kanbkam/import.sqlite3 --apply
```

The first command is a dry run. The importer is idempotent and writes changed product rows, category memberships, price-change hints, and verified direct-URL mappings. Newly linked retailer listings start paused; the importer does not resume paused listings or create direct-fetch jobs. Noon/Jumia IDs stay distinct and require verified direct URLs before retailer monitoring. To monitor a bounded selection, change its `Listing.tracking_state` to `watch` in the dashboard, set its cadence, and run `manage.py seed_deals`. The server cannot check the full imported catalog at its current source request limits.

For old auto-linked Noon/Jumia rows that were queued before this change, run `manage.py quarantine_auto_imports` for a count preview. Stop `botafflite-worker.service`, run it again with `--apply`, and restart only that worker. The command pauses only cold Kanbkam-linked listings with no successful direct check, retains their price hints and history, and leaves manually chosen normal or hot watchlists alone. See [SERVER.md](SERVER.md) for the deployed counts and backup.

The disabled tasks retain their saved cursor and data for a future server-side collection solution. Check the `completed_categories` count to see whether the full first crawl is done; do not describe the crawl as complete before that reaches `leaf_categories` and detail coverage is checked.
