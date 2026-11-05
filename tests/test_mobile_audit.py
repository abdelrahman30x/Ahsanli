"""Tests for phone-only auditing and safe baseline initialization."""

import json
import tempfile
import unittest
from pathlib import Path

from mobile_audit import audit_catalog, breadcrumb_parts, classify_phone, write_watchlist
from watcher import Settings, load_state


def product_html(title: str, breadcrumb: str) -> str:
    return (f'<div id="wayfinding-breadcrumbs_feature_div">{breadcrumb}</div>'
            f'<span id="productTitle">{title}</span>'
            '<div id="corePrice_feature_div"><span class="a-offscreen">EGP 100.00</span></div>'
            '<a id="sellerProfileTriggerId">Amazon.eg</a>')


class MobileAuditTests(unittest.TestCase):
    def test_breadcrumb_and_accessory_filter(self):
        html = product_html("Nokia Phone", "Electronics › Mobile Phones & Communication › Mobile Phones")
        parts = breadcrumb_parts(html)
        self.assertEqual(classify_phone("Nokia Phone", parts), "phone")
        self.assertEqual(classify_phone("جراب متوافق مع هاتف Nokia", parts), "other")
        self.assertEqual(classify_phone("Silicone Case for iPhone 16", parts), "other")
        self.assertEqual(classify_phone("Pure Saffron Premium 5g", parts), "other")
        self.assertEqual(classify_phone("Osmo Mobile Device Only", parts), "other")
        self.assertEqual(classify_phone("MP3 Player 16GB", parts), "other")
        self.assertEqual(classify_phone("Hope K19 4 Sim Cards", parts), "phone")
        self.assertEqual(classify_phone("Apple iPhone 16", []), "phone")
        self.assertEqual(classify_phone("Mobile Mouse", ["Computers", "Mice"]), "other")
        self.assertEqual(classify_phone("Unknown", []), "unknown")

    def test_audit_seeds_verified_prices_without_alerts(self):
        catalog = {
            "collected_at_utc": "2026-10-30T00:00:00+00:00",
            "products": [
                {"asin": "B012345678", "title": "Nokia Phone"},
                {"asin": "B012345679", "title": "Mouse"},
            ],
        }
        pages = {
            "B012345678": product_html("Nokia Phone", "Electronics › Mobile Phones"),
            "B012345679": product_html("Mouse", "Computers › Mice"),
        }
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            audit = audit_catalog(catalog, lambda url: pages[url.rsplit("/", 1)[-1]],
                                  root / "audit.json", delay=0)
            settings = Settings(
                marketplace="www.amazon.eg", partner_tag="test-21", credential_id="",
                credential_secret="", credential_version="3.2", telegram_bot_token="token",
                telegram_chat_id="@channel", source="scrape", scrape_currency="EGP",
            )
            count = write_watchlist(catalog, audit, settings, root / "urls.txt",
                                    root / "review.json", root / "prices.json")
            self.assertEqual(count, 1)
            self.assertIn("B012345678", (root / "urls.txt").read_text(encoding="utf-8"))
            self.assertNotIn("B012345679", (root / "urls.txt").read_text(encoding="utf-8"))
            state = load_state(root / "prices.json")
            self.assertEqual(state["products"]["B012345678"]["price"], "100.00")
            self.assertEqual(state["products"]["B012345678"]["pending"], [])
            self.assertEqual(json.loads((root / "review.json").read_text(encoding="utf-8"))["count"], 0)

    def test_resume_rechecks_unknown_and_reclassifies_accessory(self):
        catalog = {"collected_at_utc": "2026-10-30T00:00:00+00:00", "products": [
            {"asin": "B012345678", "title": "Silicone Case for iPhone 16"},
            {"asin": "B012345679", "title": "Nokia Phone"},
        ]}
        with tempfile.TemporaryDirectory() as directory:
            checkpoint = Path(directory) / "audit.json"
            checkpoint.write_text(json.dumps({
                "catalog_collected_at": catalog["collected_at_utc"],
                "records": {
                    "B012345678": {"classification": "phone", "breadcrumbs": ["Electronics", "Mobile Phones"]},
                    "B012345679": {"classification": "unknown", "breadcrumbs": []},
                },
            }), encoding="utf-8")
            called = []

            def fetch(url):
                called.append(url)
                return product_html("Nokia Phone", "Electronics › Mobile Phones")

            audit = audit_catalog(catalog, fetch, checkpoint, delay=0)
            self.assertEqual(len(called), 1)
            self.assertEqual(audit["records"]["B012345678"]["classification"], "other")
            self.assertEqual(audit["records"]["B012345679"]["classification"], "phone")


if __name__ == "__main__":
    unittest.main()
