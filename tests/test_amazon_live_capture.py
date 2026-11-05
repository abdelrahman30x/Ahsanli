import os
import tempfile
import unittest
from pathlib import Path

from capture_product import capture_product_page


@unittest.skipUnless(os.environ.get("RUN_LIVE_AMAZON_CAPTURE_TESTS") == "1",
                     "set RUN_LIVE_AMAZON_CAPTURE_TESTS=1 for a real Amazon screenshot")
class AmazonLiveCaptureTests(unittest.TestCase):
    def test_real_product_has_price_seller_and_screenshot(self):
        url = next(line.strip() for line in Path("urls.txt").read_text(encoding="utf-8").splitlines()
                   if line.strip() and not line.lstrip().startswith("#"))
        with tempfile.TemporaryDirectory() as directory:
            image = Path(directory) / "amazon.png"
            observed = capture_product_page(url, image, "EGP")
            self.assertGreater(float(observed["price"]), 0)
            self.assertEqual(observed["currency"], "EGP")
            self.assertTrue(observed["title"])
            self.assertTrue(observed["seller"])
            self.assertTrue(image.read_bytes().startswith(b"\x89PNG\r\n\x1a\n"))
            self.assertGreater(image.stat().st_size, 10_000)


if __name__ == "__main__":
    unittest.main()
