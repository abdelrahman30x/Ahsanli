"""Unit and integration tests for the price tracker database layer."""

import unittest
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from pathlib import Path
import tempfile
import sqlite3

from db.database import TrackerDB
from db.schema import SCHEMA_SQL, SCHEMA_VERSION


class TestTrackerDB(unittest.TestCase):
    def test_version_two_database_adds_attempt_column_without_losing_products(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "legacy.db"
            con = sqlite3.connect(path)
            try:
                con.executescript(SCHEMA_SQL.replace("    last_attempt_at TEXT,\n", ""))
                con.execute("INSERT INTO schema_version (version) VALUES (2)")
                con.execute("INSERT INTO sites (code, marketplace) VALUES ('amazon_eg', 'www.amazon.eg')")
                con.execute("INSERT INTO products (asin, site_id, current_price) VALUES ('B012345678', 1, '100')")
                con.commit()
            finally:
                con.close()
            db = TrackerDB(path)
            try:
                self.assertIn("last_attempt_at", {r[1] for r in db.conn.execute("PRAGMA table_info(products)")})
                self.assertEqual(db.conn.execute("SELECT version FROM schema_version").fetchone()[0], SCHEMA_VERSION)
                self.assertEqual(db.get_product("B012345678", 1)["current_price"], "100")
            finally:
                db.close()

    def test_existing_alert_table_gains_capture_retry_columns(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "old.db"
            con = sqlite3.connect(path)
            try:
                con.execute("""CREATE TABLE price_alerts (
                    id INTEGER PRIMARY KEY, asin TEXT NOT NULL, site_id INTEGER NOT NULL,
                    old_price TEXT NOT NULL, new_price TEXT NOT NULL, currency TEXT NOT NULL,
                    status TEXT NOT NULL DEFAULT 'pending', created_at TEXT NOT NULL, sent_at TEXT)""")
                con.execute("INSERT INTO price_alerts VALUES (1,'B012345678',1,'100','80','EGP','pending','2026-01-01T00:00:00Z',NULL)")
                con.commit()
            finally:
                con.close()
            db = TrackerDB(path)
            try:
                cols = {row[1] for row in db.conn.execute("PRAGMA table_info(price_alerts)")}
                self.assertTrue({"capture_attempts", "next_attempt_at", "last_error"} <= cols)
                self.assertEqual(db.conn.execute("SELECT capture_attempts FROM price_alerts WHERE id=1").fetchone()[0], 0)
            finally:
                db.close()

    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp_dir.cleanup)
        self.db_path = Path(self.temp_dir.name) / "test_tracker.db"
        self.db = TrackerDB(self.db_path)
        self.addCleanup(self.db.close)

        # Register default test site
        self.site_id = self.db.get_or_create_site(
            code="amazon_com",
            marketplace="www.amazon.com",
            partner_tag="testtag-20",
            currency="USD",
        )

    def test_site_creation_and_retrieval(self):
        site = self.db.get_site(self.site_id)
        self.assertIsNotNone(site)
        self.assertEqual(site["code"], "amazon_com")
        self.assertEqual(site["marketplace"], "www.amazon.com")
        self.assertEqual(site["partner_tag"], "testtag-20")

        # Idempotent get_or_create
        same_id = self.db.get_or_create_site(
            code="amazon_com",
            marketplace="www.amazon.com",
            partner_tag="updatedtag-20",
        )
        self.assertEqual(self.site_id, same_id)
        updated_site = self.db.get_site(same_id)
        self.assertEqual(updated_site["partner_tag"], "updatedtag-20")

    def test_category_management(self):
        cat_id = self.db.upsert_category(
            self.site_id,
            "Electronics",
            "https://www.amazon.com/s?k=electronics",
        )
        categories = self.db.get_active_categories(self.site_id)
        self.assertEqual(len(categories), 1)
        self.assertEqual(categories[0]["name"], "Electronics")

        # Upsert category update
        self.db.upsert_category(
            self.site_id,
            "Electronics",
            "https://www.amazon.com/s?k=electronics&page=2",
        )
        categories = self.db.get_active_categories(self.site_id)
        self.assertEqual(len(categories), 1)
        self.assertEqual(categories[0]["url"], "https://www.amazon.com/s?k=electronics&page=2")

    def test_upsert_product_and_price_history(self):
        res = self.db.upsert_product(
            asin="B08N5WRWNW",
            site_id=self.site_id,
            title="Echo Dot (4th Gen)",
            price="49.99",
            currency="USD",
            seller="Amazon.com",
            category_name="Electronics",
            affiliate_url="https://www.amazon.com/dp/B08N5WRWNW?tag=testtag-20",
        )
        self.assertTrue(res["is_new"])
        self.assertIsNone(res["alert"])  # Baseline price, no drop yet

        product = self.db.get_product("B08N5WRWNW", self.site_id)
        self.assertIsNotNone(product)
        self.assertEqual(product["current_price"], "49.99")
        self.assertEqual(product["title"], "Echo Dot (4th Gen)")

        history = self.db.get_price_history("B08N5WRWNW", self.site_id)
        self.assertEqual(len(history), 1)
        self.assertEqual(history[0]["price"], "49.99")

    def test_price_drop_triggers_alert(self):
        # 1. Establish baseline
        self.db.upsert_product(
            asin="B08N5WRWNW",
            site_id=self.site_id,
            title="Echo Dot (4th Gen)",
            price="49.99",
            currency="USD",
            seller="Amazon.com",
        )

        # 2. Hourly check: price drops to 29.99
        alert = self.db.record_price(
            asin="B08N5WRWNW",
            site_id=self.site_id,
            price="29.99",
            currency="USD",
            seller="Amazon.com",
        )

        self.assertIsNotNone(alert)
        self.assertEqual(alert["old_price"], "49.99")
        self.assertEqual(alert["new_price"], "29.99")
        self.assertEqual(alert["currency"], "USD")
        self.assertEqual(alert["savings"], "20.00")
        self.assertEqual(alert["drop_percent"], "40%")

        # Check pending alerts queue
        pending = self.db.get_pending_alerts(self.site_id)
        self.assertEqual(len(pending), 1)
        self.assertEqual(pending[0]["asin"], "B08N5WRWNW")
        self.assertEqual(pending[0]["savings"], "20.00")

        # Mark sent
        self.db.mark_alert_sent(pending[0]["alert_id"])
        self.assertEqual(len(self.db.get_pending_alerts(self.site_id)), 0)

    def test_paused_product_alert_is_not_dispatched(self):
        self.db.upsert_product(asin="B08N5WRWNW", site_id=self.site_id,
                               price="49.99", currency="USD")
        self.db.record_price(asin="B08N5WRWNW", site_id=self.site_id,
                             price="29.99", currency="USD")
        self.assertEqual(len(self.db.get_pending_alerts(self.site_id)), 1)
        self.db.set_product_tracked("B08N5WRWNW", self.site_id, tracked=False)
        self.assertEqual(self.db.get_pending_alerts(self.site_id), [])

    def test_price_increase_does_not_trigger_alert(self):
        self.db.upsert_product(
            asin="B08N5WRWNW",
            site_id=self.site_id,
            price="29.99",
            currency="USD",
        )
        # Price goes up to 39.99
        alert = self.db.record_price(
            asin="B08N5WRWNW",
            site_id=self.site_id,
            price="39.99",
            currency="USD",
        )
        self.assertIsNone(alert)
        self.assertEqual(len(self.db.get_pending_alerts(self.site_id)), 0)

        # But current price should update to 39.99
        product = self.db.get_product("B08N5WRWNW", self.site_id)
        self.assertEqual(product["current_price"], "39.99")

    def test_deduplication_and_multiple_categories(self):
        # Product seen in Electronics
        self.db.upsert_product(
            asin="B07XJ8C8F5",
            site_id=self.site_id,
            title="Fire TV Stick",
            category_name="Electronics",
            price="39.99",
            currency="USD",
        )

        # Product seen again later in Home & Kitchen
        self.db.upsert_product(
            asin="B07XJ8C8F5",
            site_id=self.site_id,
            title="Fire TV Stick 4K",
            category_name="Home & Kitchen",
            price="39.99",
            currency="USD",
        )

        # Verify only 1 product exists in DB
        self.assertEqual(self.db.count_products(self.site_id), 1)
        prod = self.db.get_product("B07XJ8C8F5", self.site_id)
        self.assertIn("Electronics", prod["category_name"])
        self.assertIn("Home & Kitchen", prod["category_name"])

    def test_products_needing_check_query(self):
        # Product checked 2 hours ago
        self.db.upsert_product(
            asin="B001",
            site_id=self.site_id,
            price="10.00",
            currency="USD",
        )
        old_time = (datetime.now(timezone.utc) - timedelta(hours=2)).isoformat(timespec="seconds")
        with self.db.conn:
            self.db.conn.execute("UPDATE products SET last_checked_at = ? WHERE asin = 'B001'", (old_time,))

        # Product checked just now
        self.db.upsert_product(
            asin="B002",
            site_id=self.site_id,
            price="20.00",
            currency="USD",
        )

        needing = self.db.get_products_needing_check(self.site_id, older_than_hours=1.0)
        asins = [p["asin"] for p in needing]
        self.assertIn("B001", asins)
        self.assertNotIn("B002", asins)

    def test_failed_attempt_rotates_and_unchecked_products_get_capacity(self):
        old_time = (datetime.now(timezone.utc) - timedelta(hours=2)).isoformat(timespec="seconds")
        for index in range(40):
            warm_asin = f"W{index:09d}"
            cold_asin = f"C{index:09d}"
            self.db.upsert_product(warm_asin, self.site_id, price="100", currency="USD")
            self.db.upsert_product(cold_asin, self.site_id)
        with self.db.conn:
            self.db.conn.execute("UPDATE products SET last_checked_at = ? WHERE asin LIKE 'W%'", (old_time,))
            self.db.conn.execute("UPDATE products SET last_checked_at = NULL WHERE asin LIKE 'C%'")

        first = self.db.get_products_needing_check(self.site_id, older_than_hours=1, limit=30)
        self.assertEqual((sum(row["asin"].startswith("W") for row in first),
                          sum(row["asin"].startswith("C") for row in first)), (24, 6))
        for row in first:
            self.db.mark_price_attempt(row["asin"], self.site_id)
        second = self.db.get_products_needing_check(self.site_id, older_than_hours=1, limit=30)
        self.assertEqual(len(second), 30)
        self.assertFalse({r["asin"] for r in first} & {r["asin"] for r in second})
        self.assertEqual(self.db.get_product(first[0]["asin"], self.site_id)["last_checked_at"], old_time)

    def test_kanbkam_daily_deals(self):
        self.db.upsert_product(
            asin="B001",
            site_id=self.site_id,
            title="Deal 1",
            price="100.00",
            currency="USD",
        )
        self.db.record_price("B001", self.site_id, "80.00", "USD")  # 20% drop

        self.db.upsert_product(
            asin="B002",
            site_id=self.site_id,
            title="Deal 2",
            price="100.00",
            currency="USD",
        )
        self.db.record_price("B002", self.site_id, "50.00", "USD")  # 50% drop

        deals = self.db.get_daily_deals(self.site_id, min_drop_percent=10.0)
        self.assertEqual(len(deals), 2)
        # Check sorted by highest discount first
        self.assertEqual(deals[0]["asin"], "B002")
        self.assertEqual(deals[0]["drop_percent"], "50%")
        self.assertEqual(deals[1]["asin"], "B001")
        self.assertEqual(deals[1]["drop_percent"], "20%")

    def test_scrape_jobs_logging(self):
        job_id = self.db.start_scrape_job("category_scan", "Electronics")
        self.assertGreater(job_id, 0)
        self.db.finish_scrape_job(
            job_id,
            status="done",
            products_found=48,
            products_updated=10,
            price_drops=2,
            errors=0,
        )

        stats = self.db.get_stats()
        self.assertIn("products", stats)
        self.assertIn("categories", stats)

    def test_out_of_stock_and_restock_alert(self):
        self.db.upsert_product(
            asin="B0RESTOCK",
            site_id=self.site_id,
            title="Restock Item",
            price="100.00",
            currency="USD",
        )
        prod = self.db.get_product("B0RESTOCK", self.site_id)
        self.assertEqual(prod["in_stock"], 1)

        # Mark out of stock
        self.db.mark_product_out_of_stock("B0RESTOCK", self.site_id)
        prod_oos = self.db.get_product("B0RESTOCK", self.site_id)
        self.assertEqual(prod_oos["in_stock"], 0)

        # Product comes back in stock at higher price ($120)
        alert = self.db.record_price("B0RESTOCK", self.site_id, "120.00", "USD")
        self.assertIsNotNone(alert)
        self.assertTrue(alert.get("is_restock"))
        self.assertEqual(alert["new_price"], "120.00")

        # Verify in_stock reset to 1
        prod_restocked = self.db.get_product("B0RESTOCK", self.site_id)
        self.assertEqual(prod_restocked["in_stock"], 1)
        self.assertEqual(prod_restocked["current_price"], "120.00")


if __name__ == "__main__":
    unittest.main()
