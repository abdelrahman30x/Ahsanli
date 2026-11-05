"""Unit tests for category_scraper.py using HTML fixtures."""

import unittest
from pathlib import Path

from amazon_scraper import ScrapeError
from category_scraper import (
    CategoryScraper,
    build_page_url,
    parse_category_page_html,
)
from request_policy import RequestGate
from test_request_policy import FakeClock

FIXTURES_DIR = Path(__file__).parent / "fixtures"


def load_fixture(name: str) -> str:
    return (FIXTURES_DIR / name).read_text(encoding="utf-8")


class TestCategoryScraper(unittest.TestCase):
    def test_parse_category_page_products(self):
        html = load_fixture("category_listing.html")
        result = parse_category_page_html(html, base_url="https://www.amazon.com", default_currency="USD")

        self.assertEqual(len(result.products), 4)
        self.assertTrue(result.has_next_page)
        self.assertIn("page=2", result.next_page_url)

        # Check Product 1
        p1 = result.products[0]
        self.assertEqual(p1.asin, "B08N5WRWNW")
        self.assertIn("Echo Dot (4th Gen)", p1.title)
        self.assertEqual(p1.price, "29.99")
        self.assertEqual(p1.currency, "USD")
        self.assertEqual(p1.rating, 4.7)
        self.assertEqual(p1.review_count, 154230)
        self.assertFalse(p1.is_sponsored)

        # Check Product 2
        p2 = result.products[1]
        self.assertEqual(p2.asin, "B09B8W5FW7")
        self.assertEqual(p2.price, "249.50")
        self.assertEqual(p2.rating, 4.3)
        self.assertEqual(p2.review_count, 3420)

        # Check Product 3 (Sponsored)
        p3 = result.products[2]
        self.assertEqual(p3.asin, "B07XJ8C8F5")
        self.assertEqual(p3.price, "39.99")
        self.assertTrue(p3.is_sponsored)

        # Check Product 4 (EGP currency detection)
        p4 = result.products[3]
        self.assertEqual(p4.asin, "B0CL123456")
        self.assertEqual(p4.price, "850.00")
        self.assertEqual(p4.currency, "EGP")

    def test_parse_last_page(self):
        html = load_fixture("category_listing_last_page.html")
        result = parse_category_page_html(html, base_url="https://www.amazon.com", default_currency="USD")

        self.assertEqual(len(result.products), 1)
        self.assertEqual(result.products[0].asin, "B00FINAL01")
        self.assertFalse(result.has_next_page)
        self.assertIsNone(result.next_page_url)

    def test_robot_check_error_raised(self):
        html = load_fixture("category_listing_captcha.html")
        with self.assertRaisesRegex(ScrapeError, "CAPTCHA"):
            parse_category_page_html(html)

    def test_build_page_url(self):
        base = "https://www.amazon.com/s?k=laptop&ref=nb_sb_noss"
        p2 = build_page_url(base, 2)
        self.assertIn("page=2", p2)
        self.assertIn("k=laptop", p2)

        p3 = build_page_url(p2, 3)
        self.assertIn("page=3", p3)
        self.assertNotIn("page=2", p3)

    def test_category_scraper_multi_page(self):
        pages = {
            1: load_fixture("category_listing.html"),
            2: load_fixture("category_listing_last_page.html"),
        }

        def fake_fetcher(url: str) -> str:
            if "page=2" in url:
                return pages[2]
            return pages[1]

        scraper = CategoryScraper(
            base_url="https://www.amazon.com",
            default_currency="USD",
            request_delay_range=(0, 0),
            fetcher=fake_fetcher,
        )

        products = scraper.scrape_category("https://www.amazon.com/s?k=test", max_pages=5)
        # 4 from page 1 + 1 from page 2 = 5 total unique products
        self.assertEqual(len(products), 5)
        asins = [p.asin for p in products]
        self.assertIn("B08N5WRWNW", asins)
        self.assertIn("B00FINAL01", asins)

    def test_category_pages_share_request_rate_limit(self):
        clock = FakeClock()
        requests = []
        gate = RequestGate(2, 60, clock=clock.now, sleep=clock.sleep,
                           jitter=lambda _start, _end: 0.0)

        def fetch(url):
            requests.append(clock.now())
            return load_fixture("category_listing_last_page.html" if "page=2" in url
                                else "category_listing.html")

        scraper = CategoryScraper(base_url="https://www.amazon.com",
                                  request_delay_range=(0, 0), fetcher=fetch,
                                  request_gate=gate)
        scraper.scrape_category("https://www.amazon.com/s?k=test", max_pages=2)
        self.assertEqual(requests, [0.0, 30.0])


if __name__ == "__main__":
    unittest.main()
