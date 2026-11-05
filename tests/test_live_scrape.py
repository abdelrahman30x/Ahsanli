"""Opt-in real product-page check. Set RUN_LIVE_SCRAPE_TESTS=1 to run."""

import os
import unittest
from pathlib import Path

from watcher import ScraperClient, Settings, read_asins


@unittest.skipUnless(os.environ.get("RUN_LIVE_SCRAPE_TESTS") == "1",
                     "requires RUN_LIVE_SCRAPE_TESTS=1 and real settings/URLs")
class LiveScrapeTests(unittest.TestCase):
    def test_product_page_has_a_price(self):
        settings = Settings.from_file(Path("settings.json"))
        self.assertEqual(settings.source, "scrape")
        asin = read_asins(Path("urls.txt"), settings.marketplace)[0]
        result = ScraperClient(settings).get_items([asin])
        self.assertIn(asin, result, "Amazon blocked the request or the page has no recognized price and seller")
        listing = result[asin]["offersV2"]["listings"][0]
        self.assertTrue(listing["merchantInfo"]["name"])
        self.assertTrue(listing["price"]["money"]["amount"])


if __name__ == "__main__":
    unittest.main()
