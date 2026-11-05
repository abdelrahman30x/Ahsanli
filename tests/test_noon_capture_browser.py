import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from noon_capture import NoonCaptureError, _browser_launch_options, fetch_noon_product


class NoonBrowserLaunchTests(unittest.TestCase):
    def test_linux_uses_virtual_display_without_headless_mode(self):
        with patch("noon_capture.sys.platform", "linux"), patch.dict(os.environ, {"DISPLAY": ":99"}):
            self.assertEqual(_browser_launch_options(""), {"headless": False})

    def test_linux_without_display_has_setup_instructions(self):
        with patch("noon_capture.sys.platform", "linux"), patch.dict(os.environ, {"DISPLAY": ""}):
            with self.assertRaisesRegex(NoonCaptureError, "xvfb-run"):
                _browser_launch_options("")

    def test_windows_keeps_browser_off_screen(self):
        with patch("noon_capture.sys.platform", "win32"):
            self.assertEqual(_browser_launch_options(""), {
                "headless": False, "args": ["--window-position=-32000,-32000"]})


@unittest.skipUnless(os.environ.get("RUN_BROWSER_TESTS") == "1",
                     "set RUN_BROWSER_TESTS=1 to launch local Chromium")
class NoonCaptureBrowserTests(unittest.TestCase):
    def test_cropped_image_has_current_price_and_seller(self):
        fixture = Path(__file__).parent / "fixtures" / "noon_product_visible.html"
        with tempfile.TemporaryDirectory() as directory:
            image = Path(directory) / "noon.png"
            result = fetch_noon_product(fixture.resolve().as_uri(), output=image)
            self.assertEqual(result["price"], "310.00")
            self.assertEqual(result["seller"], "Sample Seller")
            self.assertTrue(image.exists())
            self.assertGreater(image.stat().st_size, 1000)

    def test_current_noon_layout_has_price_seller_and_photo(self):
        fixture = Path(__file__).parent / "fixtures" / "noon_product_current_layout.html"
        with tempfile.TemporaryDirectory() as directory:
            image = Path(directory) / "noon.png"
            result = fetch_noon_product(fixture.resolve().as_uri(), output=image)
            self.assertEqual(result["price"], "315.00")
            self.assertEqual(result["seller"], "X-Tech")
            self.assertTrue(image.exists())
            self.assertGreater(image.stat().st_size, 1000)


if __name__ == "__main__":
    unittest.main()
