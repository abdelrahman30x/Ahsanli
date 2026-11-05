"""Opt-in live API and Telegram smoke test. Set RUN_LIVE_TESTS=1 to run."""

import os
import unittest
from pathlib import Path

from watcher import AmazonClient, Settings, TelegramNotifier, read_asins


@unittest.skipUnless(os.environ.get("RUN_LIVE_TESTS") == "1", "requires RUN_LIVE_TESTS=1 and real credentials")
class LiveServicesTests(unittest.TestCase):
    def test_amazon_price_and_telegram_delivery(self):
        settings = Settings.from_file(Path("settings.json"))
        asin = read_asins(Path("urls.txt"), settings.marketplace)[0]
        item = AmazonClient(settings).get_items([asin])[asin]
        self.assertTrue(item.get("offersV2", {}).get("listings"), "Product has no offer listings")
        TelegramNotifier(settings).send(f"Amazon price watcher live test passed for {asin}.")


if __name__ == "__main__":
    unittest.main()
