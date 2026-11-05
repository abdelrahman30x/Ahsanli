"""Unit tests for price_checker.py with simulated price updates and alerts."""

import unittest
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timedelta, timezone
from pathlib import Path
from threading import Event
import tempfile
from types import SimpleNamespace
from unittest.mock import patch

from capture_product import CaptureError
from config import TrackerConfig
from db.database import TrackerDB
from price_checker import PriceChecker, PriceCheckError
from request_policy import RequestGate
from test_request_policy import FakeClock

FIXTURES_DIR = Path(__file__).parent / "fixtures"


def load_fixture(name: str) -> str:
    return (FIXTURES_DIR / name).read_text(encoding="utf-8")


class FakeTelegramNotifier:
    def __init__(self):
        self.sent_messages: list[str] = []

    def send(self, message: str) -> None:
        self.sent_messages.append(message)


class PhotoNotifier(FakeTelegramNotifier):
    def __init__(self):
        super().__init__()
        self.photos: list[tuple[bytes, str]] = []

    def send_photo(self, path: Path, caption: str) -> None:
        self.photos.append((path.read_bytes(), caption))

    def send_photo_url(self, _url: str, _caption: str) -> None:
        raise AssertionError("Catalog product images must not be posted")


class TestPriceChecker(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp_dir.cleanup)
        self.db_path = Path(self.temp_dir.name) / "test_tracker.db"
        self.db = TrackerDB(self.db_path)
        self.addCleanup(self.db.close)

        self.site_id = self.db.get_or_create_site(
            code="amazon_eg",
            marketplace="www.amazon.eg",
            partner_tag="affiliate-20",
            currency="EGP",
        )
        self.notifier = FakeTelegramNotifier()

    def test_hourly_price_check_detects_drop_and_sends_telegram_alert(self):
        # 1. Product initial state: 1399.00 EGP
        self.db.upsert_product(
            asin="B012345678",
            site_id=self.site_id,
            title="Sample Headphones",
            price="1399.00",
            currency="EGP",
            seller="ShopOne",
            affiliate_url="https://www.amazon.eg/dp/B012345678?tag=affiliate-20",
        )

        # 2. Fake fetcher returns product_drop.html ($1199.00)
        def fake_fetcher(url: str, proxy_url: str = "") -> str:
            return load_fixture("product_drop.html")

        notifier = PhotoNotifier()
        checker = PriceChecker(
            db=self.db,
            notifier=notifier,
            fetcher=fake_fetcher,
            request_delay=0.0,
        )

        # 3. Run check
        def capture(_url, output, _currency):
            output.write_bytes(b"\x89PNG\r\n\x1a\npage")
            return {"price": "1199.00", "seller": "ShopTwo"}
        with patch("capture_product.capture_product_page", side_effect=capture):
            stats = checker.check_site_prices(self.site_id, older_than_hours=0.0)
        self.assertEqual(stats["checked"], 1)
        self.assertEqual(stats["drops"], 1)
        self.assertEqual(stats["errors"], 0)

        # 4. Check the alert was sent as a photo.
        self.assertEqual(len(notifier.photos), 1)
        self.assertEqual(notifier.sent_messages, [])
        msg = notifier.photos[0][1]
        self.assertIn("السعر نزل", msg)
        self.assertIn("1,199.00 جنيه", msg)
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 14.3%", msg)
        self.assertNotIn("1,399.00 جنيه", msg)

        # 5. Alert status is now marked 'sent'
        pending = self.db.get_pending_alerts(self.site_id)
        self.assertEqual(len(pending), 0)

        # 6. Database current price is updated
        prod = self.db.get_product("B012345678", self.site_id)
        self.assertEqual(prod["current_price"], "1199.00")

    def test_same_price_generates_no_alert(self):
        self.db.upsert_product(
            asin="B012345678",
            site_id=self.site_id,
            title="Sample Headphones",
            price="1299.00",
            currency="EGP",
            seller="ShopOne",
        )

        def fake_fetcher(url: str, proxy_url: str = "") -> str:
            return load_fixture("product_visible.html")  # price is 1299.00 EGP

        checker = PriceChecker(
            db=self.db,
            notifier=self.notifier,
            fetcher=fake_fetcher,
            request_delay=0.0,
        )

        stats = checker.check_site_prices(self.site_id, older_than_hours=0.0)
        self.assertEqual(stats["checked"], 1)
        self.assertEqual(stats["drops"], 0)
        self.assertEqual(len(self.notifier.sent_messages), 0)

    def test_scrape_failure_handled_gracefully(self):
        self.db.upsert_product(
            asin="B012345678",
            site_id=self.site_id,
            price="1299.00",
            currency="EGP",
        )
        old_time = (datetime.now(timezone.utc) - timedelta(hours=2)).isoformat(timespec="seconds")
        with self.db.conn:
            self.db.conn.execute("UPDATE products SET last_checked_at = ? WHERE asin = 'B012345678'", (old_time,))

        def fake_fetcher(url: str, proxy_url: str = "") -> str:
            return load_fixture("robot_check.html")  # Causes ScrapeError

        checker = PriceChecker(
            db=self.db,
            notifier=self.notifier,
            fetcher=fake_fetcher,
            request_delay=0.0,
        )

        stats = checker.check_site_prices(self.site_id, older_than_hours=0.0)
        self.assertEqual(stats["checked"], 0)
        self.assertEqual(stats["errors"], 1)
        self.assertEqual(len(self.notifier.sent_messages), 0)

        # Original price preserved
        prod = self.db.get_product("B012345678", self.site_id)
        self.assertEqual(prod["current_price"], "1299.00")
        self.assertEqual(prod["last_checked_at"], old_time)
        self.assertIsNotNone(prod["last_attempt_at"])
        self.assertEqual(self.db.get_products_needing_check(self.site_id, older_than_hours=1), [])

    def test_configured_rate_caps_one_run_and_spaces_requests(self):
        for index in range(3):
            self.db.upsert_product(asin=f"B00000000{index}", site_id=self.site_id,
                                   price="1299.00", currency="EGP", seller="ShopOne")
        clock = FakeClock()
        calls = []

        def fetch(_url, proxy_url=""):
            calls.append(clock.now())
            return load_fixture("product_visible.html")

        config = TrackerConfig(max_requests_per_minute=2, max_price_checks_per_run=2)
        checker = PriceChecker(self.db, fetcher=fetch, config=config, request_delay=0)
        checker._request_gates[self.site_id] = RequestGate(
            2, 60, clock=clock.now, sleep=clock.sleep,
            jitter=lambda _start, _end: 0.0)
        stats = checker.check_site_prices(self.site_id, older_than_hours=0, max_workers=5)
        self.assertEqual((stats["checked"], stats["errors"]), (2, 0))
        self.assertEqual(calls, [0.0, 30.0])

    def test_repeated_http_blocks_pause_site_without_scanning_rest(self):
        for index in range(12):
            self.db.upsert_product(asin=f"B0000000{index:02d}", site_id=self.site_id,
                                   price="1299.00", currency="EGP", seller="ShopOne")
        clock = FakeClock()
        calls = []

        def blocked(_url, proxy_url=""):
            calls.append(clock.now())
            raise PriceCheckError("HTTP 503 fetching product page")

        config = TrackerConfig(max_requests_per_minute=2, max_price_checks_per_run=12,
                               block_cooldown_minutes=60)
        checker = PriceChecker(self.db, fetcher=blocked, config=config)
        checker._request_gates[self.site_id] = RequestGate(
            2, 60, clock=clock.now, sleep=clock.sleep,
            jitter=lambda _start, _end: 0.0)
        stats = checker.check_site_prices(self.site_id, older_than_hours=0, max_workers=5)
        self.assertEqual(stats["errors"], 3)
        self.assertEqual(len(calls), 3)
        self.assertEqual(checker.check_site_prices(self.site_id, older_than_hours=0)["errors"], 0)
        self.assertEqual(len(calls), 3)

    def test_cloudscraper_client_reads_page_and_closes_session(self):
        config = TrackerConfig(http_client="cloudscraper")
        checker = PriceChecker(self.db, config=config)
        url = "https://www.amazon.eg/dp/B012345678"
        content = load_fixture("product_visible.html")
        response = SimpleNamespace(status_code=200, url=url, content=content.encode(), text=content)
        session = SimpleNamespace(get=lambda *_args, **_kwargs: response, close=lambda: None)
        with patch.dict("sys.modules", {"cloudscraper": SimpleNamespace(create_scraper=lambda: session)}):
            self.assertEqual(checker._default_fetcher(url), content)

    def test_drop_posts_verified_page_screenshot_instead_of_catalog_image(self):
        self.db.upsert_product(
            asin="B012345678", site_id=self.site_id, title="Sample Headphones",
            image_url="https://example.com/product.jpg", price="1399.00",
            currency="EGP", seller="ShopOne",
        )
        self.db.record_price("B012345678", self.site_id, "1199.00", "EGP", "ShopOne")
        notifier = PhotoNotifier()
        checker = PriceChecker(self.db, notifier=notifier)
        captured_urls = []

        def capture(url, output, currency):
            captured_urls.append((url, currency))
            output.write_bytes(b"\x89PNG\r\n\x1a\npage screenshot")
            return {"price": "1199.00", "seller": "ShopOne", "store_discount_percent": "21%",
                    "stock_count": 3}

        with patch("capture_product.capture_product_page", side_effect=capture):
            self.assertEqual(checker.dispatch_pending_alerts(self.site_id), 1)
        self.assertEqual(captured_urls, [("https://www.amazon.eg/dp/B012345678", "EGP")])
        self.assertEqual(len(notifier.photos), 1)
        self.assertTrue(notifier.photos[0][0].startswith(b"\x89PNG"))
        self.assertIn("ShopOne", notifier.photos[0][1])
        self.assertIn("الخصم المعروض على صفحة أمازون: 21%", notifier.photos[0][1])
        self.assertIn("آخر 3 قطع", notifier.photos[0][1])
        self.assertNotIn("14.3%", notifier.photos[0][1])
        self.assertEqual(notifier.sent_messages, [])
        self.assertEqual(self.db.get_pending_alerts(self.site_id), [])

    def test_mismatched_price_or_failed_page_capture_never_sends_text(self):
        self.db.upsert_product(
            asin="B012345678", site_id=self.site_id, price="1399.00",
            currency="EGP", seller="ShopOne",
        )
        self.db.record_price("B012345678", self.site_id, "1199.00", "EGP", "ShopOne")
        notifier = PhotoNotifier()
        checker = PriceChecker(self.db, notifier=notifier)
        with patch("capture_product.capture_product_page",
                   return_value={"price": "1198.00", "seller": "ShopOne",
                                 "store_discount_percent": "21%", "stock_count": 3}):
            self.assertEqual(checker.dispatch_pending_alerts(self.site_id), 0)
        self.assertEqual(notifier.photos, [])
        self.assertEqual(notifier.sent_messages, [])
        row = self.db.conn.execute("SELECT status,capture_attempts,next_attempt_at FROM price_alerts ORDER BY id LIMIT 1").fetchone()
        self.assertEqual((row["status"], row["capture_attempts"]), ("pending", 1))
        self.assertIsNotNone(row["next_attempt_at"])

        self.db.record_price("B012345678", self.site_id, "999.00", "EGP", "ShopOne")
        with patch("capture_product.capture_product_page", side_effect=CaptureError("blocked")):
            self.assertEqual(checker.dispatch_pending_alerts(self.site_id), 0)
        self.assertEqual(notifier.photos, [])
        self.assertEqual(notifier.sent_messages, [])

    def test_capture_is_held_after_three_attempts(self):
        self.db.upsert_product(asin="B012345678", site_id=self.site_id,
                               price="100.00", currency="EGP", seller="ShopOne")
        self.db.record_price("B012345678", self.site_id, "80.00", "EGP", "ShopOne")
        notifier = PhotoNotifier()
        checker = PriceChecker(self.db, notifier=notifier)
        with patch("capture_product.capture_product_page", side_effect=CaptureError("blocked")):
            for attempt in range(3):
                self.assertEqual(checker.dispatch_pending_alerts(self.site_id), 0)
                if attempt < 2:
                    self.db.conn.execute("UPDATE price_alerts SET next_attempt_at=NULL")
                    self.db.conn.commit()
        row = self.db.conn.execute("SELECT status,capture_attempts FROM price_alerts").fetchone()
        self.assertEqual((row["status"], row["capture_attempts"]), ("failed", 3))
        self.assertEqual(notifier.photos, [])
        self.assertEqual(notifier.sent_messages, [])

    def test_photo_upload_failure_is_uncertain_without_text_retry(self):
        self.db.upsert_product(asin="B012345678", site_id=self.site_id,
                               price="100.00", currency="EGP", seller="ShopOne")
        self.db.record_price("B012345678", self.site_id, "80.00", "EGP", "ShopOne")
        class FailingPhotoNotifier(PhotoNotifier):
            def send_photo(self, path, caption):
                raise TimeoutError("upload timed out")
        notifier = FailingPhotoNotifier()
        checker = PriceChecker(self.db, notifier=notifier)
        def capture(_url, output, _currency):
            output.write_bytes(b"\x89PNG\r\n\x1a\npage")
            return {"price": "80.00", "seller": "ShopOne"}
        with patch("capture_product.capture_product_page", side_effect=capture):
            self.assertEqual(checker.dispatch_pending_alerts(self.site_id), 0)
        self.assertEqual(self.db.conn.execute("SELECT status FROM price_alerts").fetchone()[0], "uncertain")
        self.assertEqual(notifier.sent_messages, [])
        self.assertEqual(checker.dispatch_pending_alerts(self.site_id), 0)

    def test_small_drop_is_recorded_but_never_published_or_captured(self):
        self.db.upsert_product(
            asin="B012345678", site_id=self.site_id, title="Expensive tablet",
            price="3500.00", currency="EGP", seller="ShopOne",
        )
        self.db.record_price("B012345678", self.site_id, "3485.00", "EGP", "ShopOne")
        notifier = PhotoNotifier()
        checker = PriceChecker(self.db, notifier=notifier)
        with patch("capture_product.capture_product_page") as capture:
            self.assertEqual(checker.dispatch_pending_alerts(self.site_id), 0)
            capture.assert_not_called()
        self.assertEqual(notifier.photos, [])
        self.assertEqual(notifier.sent_messages, [])
        self.assertEqual(self.db.get_pending_alerts(self.site_id), [])
        self.assertEqual(self.db.get_product("B012345678", self.site_id)["current_price"], "3485.00")
        self.assertEqual(self.db.get_price_history("B012345678", self.site_id)[0]["price"], "3485.00")
        status = self.db.conn.execute("SELECT status FROM price_alerts").fetchone()[0]
        self.assertEqual(status, "filtered")

    def test_restock_does_not_bypass_the_price_drop_threshold(self):
        self.db.upsert_product(asin="B012345678", site_id=self.site_id,
                               price="100.00", currency="EGP", seller="ShopOne")
        self.db.mark_product_out_of_stock("B012345678", self.site_id)
        self.db.record_price("B012345678", self.site_id, "95.00", "EGP", "ShopOne")
        pending = [{**alert, "is_restock": True} for alert in self.db.get_pending_alerts(self.site_id)]
        notifier = PhotoNotifier()
        checker = PriceChecker(self.db, notifier=notifier)
        with patch.object(self.db, "get_pending_alerts", return_value=pending), \
             patch("capture_product.capture_product_page") as capture:
            self.assertEqual(checker.dispatch_pending_alerts(self.site_id), 0)
            capture.assert_not_called()
        self.assertEqual(notifier.photos, [])
        self.assertEqual(self.db.conn.execute("SELECT status FROM price_alerts").fetchone()[0], "filtered")

    def test_configured_threshold_can_be_stricter(self):
        self.db.upsert_product(
            asin="B012345678", site_id=self.site_id, price="1000.00", currency="EGP",
        )
        self.db.record_price("B012345678", self.site_id, "880.00", "EGP")
        config = TrackerConfig(min_alert_drop_percent=15.0)
        checker = PriceChecker(self.db, notifier=self.notifier, config=config)
        self.assertEqual(checker.dispatch_pending_alerts(self.site_id), 0)
        self.assertEqual(self.notifier.sent_messages, [])

    def test_concurrent_dispatch_does_not_send_duplicate_alert(self):
        self.db.upsert_product(
            asin="B012345678", site_id=self.site_id, title="Sample Headphones",
            price="100.00", currency="EGP", seller="ShopOne",
        )
        self.db.record_price("B012345678", self.site_id, "80.00", "EGP", "ShopOne")
        entered = Event()
        release = Event()

        class BlockingNotifier(PhotoNotifier):
            def send_photo(self, path, caption):
                entered.set()
                if not release.wait(timeout=5):
                    raise AssertionError("Timed out waiting for the other dispatcher")
                super().send_photo(path, caption)

        notifier = BlockingNotifier()
        checker = PriceChecker(self.db, notifier=notifier)
        def capture(_url, output, _currency):
            output.write_bytes(b"\x89PNG\r\n\x1a\npage")
            return {"price": "80.00", "seller": "ShopOne"}
        with patch("capture_product.capture_product_page", side_effect=capture), ThreadPoolExecutor(max_workers=2) as executor:
            first = executor.submit(checker.dispatch_pending_alerts, self.site_id)
            self.assertTrue(entered.wait(timeout=5))
            second = executor.submit(checker.dispatch_pending_alerts, self.site_id, blocking=False)
            self.assertEqual(second.result(timeout=5), 0)
            release.set()
            self.assertEqual(first.result(timeout=5), 1)
        self.assertEqual(len(notifier.photos), 1)
        self.assertEqual(notifier.sent_messages, [])
        self.assertEqual(checker.dispatch_pending_alerts(self.site_id), 0)


if __name__ == "__main__":
    unittest.main()
