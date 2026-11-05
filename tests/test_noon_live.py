import os
import tempfile
import unittest
from pathlib import Path

from noon_capture import fetch_noon_product
from noon_watcher import read_noon_products


@unittest.skipUnless(os.environ.get("RUN_LIVE_NOON_TESTS") == "1",
                     "set RUN_LIVE_NOON_TESTS=1 for a real Noon product read")
class NoonLiveTests(unittest.TestCase):
    def test_real_product_has_current_price_seller_and_screenshot(self):
        product = read_noon_products(Path("noon_urls.txt"))[0]
        with tempfile.TemporaryDirectory() as directory:
            image = Path(directory) / "noon.png"
            observed = fetch_noon_product(product.url, output=image)
            self.assertGreater(float(observed["price"]), 0)
            self.assertEqual(observed["currency"], "EGP")
            self.assertTrue(observed["title"])
            self.assertTrue(observed["seller"])
            self.assertTrue(image.read_bytes().startswith(b"\x89PNG\r\n\x1a\n"))
            self.assertGreater(image.stat().st_size, 10_000)


if __name__ == "__main__":
    unittest.main()
