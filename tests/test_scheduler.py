"""Unit tests for the background scheduler daemon."""

import tempfile
import unittest
from unittest import mock
from datetime import datetime, timezone
from pathlib import Path

from config import SiteConfig, TrackerConfig
from scheduler import TrackerScheduler


class TestScheduler(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp_dir.cleanup)
        self.db_path = str(Path(self.temp_dir.name) / "test_tracker.db")

        self.config = TrackerConfig(
            db_path=self.db_path,
            telegram_bot_token="",
            telegram_chat_id="",
            price_check_interval_hours=1.0,
            category_scan_hour=0,
            sites=[
                SiteConfig(
                    code="amazon_test",
                    marketplace="www.amazon.com",
                    partner_tag="testtag-20",
                    currency="USD",
                    categories={
                        "Electronics": "https://www.amazon.com/s?k=electronics",
                    },
                )
            ],
        )
        self.scheduler = TrackerScheduler(self.config)
        self.addCleanup(self.scheduler.db.close)

    def test_sync_sites_and_categories_on_init(self):
        site = self.scheduler.db.get_site_by_code("amazon_test")
        self.assertIsNotNone(site)
        self.assertEqual(site["marketplace"], "www.amazon.com")

        categories = self.scheduler.db.get_active_categories(site["id"])
        self.assertEqual(len(categories), 1)
        self.assertEqual(categories[0]["name"], "Electronics")

    def test_should_run_daily_scan_at_midnight(self):
        # 1. At 00:05 UTC (hour 0), should run
        t1 = datetime(2026, 10, 25, 0, 5, tzinfo=timezone.utc)
        self.assertTrue(self.scheduler._should_run_daily_scan(t1))

        # Mark scanned for today
        self.scheduler._last_category_crawl_date = "2026-10-25"
        self.assertFalse(self.scheduler._should_run_daily_scan(t1))

        # Next day at 00:01 UTC, should run again!
        t2 = datetime(2026, 10, 26, 0, 1, tzinfo=timezone.utc)
        self.assertTrue(self.scheduler._should_run_daily_scan(t2))

    def test_run_category_scan_with_mocked_scraper(self):
        # Insert test product via mock
        from category_scraper import ScrapedCategoryProduct

        mock_products = [
            ScrapedCategoryProduct(
                asin="B0TEST0001",
                title="Mock Speaker",
                price="19.99",
                currency="USD",
                image_url="https://example.com/img.jpg",
                rating=4.5,
                review_count=100,
                url="https://www.amazon.com/dp/B0TEST0001",
            )
        ]

        def fake_scrape(url, max_pages=10):
            return mock_products

        with unittest.mock.patch("category_scraper.CategoryScraper.scrape_category", side_effect=fake_scrape):
            result = self.scheduler.run_category_scan(site_code="amazon_test", deep_subcategories=False)
            self.assertEqual(result["discovered"], 1)
            self.assertEqual(result["new"], 1)

            # Check DB has it
            site = self.scheduler.db.get_site_by_code("amazon_test")
            prod = self.scheduler.db.get_product("B0TEST0001", site["id"])
            self.assertIsNotNone(prod)
            self.assertEqual(prod["title"], "Mock Speaker")
            self.assertEqual(prod["current_price"], "19.99")
            self.assertIn("tag=testtag-20", prod["affiliate_url"])

    def test_due_photo_retries_run_between_hourly_price_checks(self):
        self.scheduler._last_category_crawl_date = datetime.now(timezone.utc).strftime("%Y-%m-%d")
        clock = iter(range(0, 1000, 61))

        def end_first_loop(timeout):
            self.scheduler._stop_event.set()

        with mock.patch("scheduler.time.monotonic", side_effect=lambda: next(clock)), \
             mock.patch.object(self.scheduler, "run_hourly_price_checks"), \
             mock.patch.object(self.scheduler.price_checker, "dispatch_pending_alerts", return_value=0) as dispatch, \
             mock.patch.object(self.scheduler._stop_event, "wait", side_effect=end_first_loop):
            self.scheduler.start(run_initial_scan=False)

        self.assertIn(mock.call(blocking=False), dispatch.call_args_list)

    def test_restart_uses_recent_completed_jobs_instead_of_rescanning(self):
        for kind in ("category_scan", "price_check"):
            job_id = self.scheduler.db.start_scrape_job(kind)
            self.scheduler.db.finish_scrape_job(job_id, status="done")
        restarted = TrackerScheduler(self.config)
        self.addCleanup(restarted.db.close)
        self.assertFalse(restarted._should_run_daily_scan(datetime.now(timezone.utc)))

        def end_first_loop(timeout):
            restarted._stop_event.set()

        with mock.patch.object(restarted, "run_hourly_price_checks") as hourly, \
             mock.patch.object(restarted._stop_event, "wait", side_effect=end_first_loop):
            restarted.start(run_initial_scan=False)
        hourly.assert_not_called()


if __name__ == "__main__":
    unittest.main()
