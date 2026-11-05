"""Unit and integration test for multi-threaded scraping and price checking."""

import tempfile
import unittest
from concurrent.futures import ThreadPoolExecutor
from decimal import Decimal
from pathlib import Path

from db.database import TrackerDB
from price_checker import PriceChecker


FIXTURES_DIR = Path(__file__).parent / "fixtures"


def load_fixture(name: str) -> str:
    return (FIXTURES_DIR / name).read_text(encoding="utf-8")


class TestMultiThreading(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp_dir.cleanup)
        self.db_path = str(Path(self.temp_dir.name) / "test_multithread.db")
        self.db = TrackerDB(self.db_path)
        self.addCleanup(self.db.close)

        self.site_id = self.db.get_or_create_site(
            code="amazon_test",
            marketplace="www.amazon.eg",
            currency="EGP",
        )

    def test_concurrent_upserts_and_price_checks(self):
        """Simulate 20 concurrent threads inserting and updating products."""
        num_products = 30

        def worker_insert(idx: int):
            asin = f"B0000000{idx:02d}"
            self.db.upsert_product(
                asin=asin,
                site_id=self.site_id,
                title=f"Concurrent Product {idx}",
                price="1499.00",
                currency="EGP",
                category_name="Electronics",
            )

        with ThreadPoolExecutor(max_workers=8) as executor:
            list(executor.map(worker_insert, range(num_products)))

        # Verify all products inserted accurately without deadlocks
        self.assertEqual(self.db.count_products(self.site_id), num_products)

        # Now simulate parallel price checks using product_drop.html (1199.00 EGP)
        def fake_fetcher(url: str, proxy_url: str = "") -> str:
            return load_fixture("product_drop.html")

        checker = PriceChecker(
            db=self.db,
            fetcher=fake_fetcher,
            request_delay=0.0,
        )

        stats = checker.check_site_prices(
            site_id=self.site_id,
            older_than_hours=0.0,
            max_workers=6,
        )

        self.assertEqual(stats["checked"], num_products)
        self.assertEqual(stats["drops"], num_products)
        self.assertEqual(stats["errors"], 0)

        # Verify pending alerts in queue
        alerts = self.db.get_pending_alerts(self.site_id, limit=100)
        self.assertEqual(len(alerts), num_products)


if __name__ == "__main__":
    unittest.main()
