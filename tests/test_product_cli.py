import contextlib
import io
import unittest
from unittest.mock import patch

import test_product
from watcher import Settings


class ProductCommandTests(unittest.TestCase):
    def test_sample_product_reports_price_and_seller(self):
        item = {
            "itemInfo": {"title": {"displayValue": "Sample phone"}},
            "offersV2": {"listings": [{
                "price": {"money": {"amount": "460.00", "currency": "EGP"}},
                "merchantInfo": {"name": "Sample seller"},
            }]},
        }
        output = io.StringIO()
        with patch("test_product.ScraperClient._get_item", return_value=item) as get_item:
            with contextlib.redirect_stdout(output):
                self.assertEqual(test_product.main([]), 0)
        get_item.assert_called_once_with("B0CPT6RBQY")
        self.assertIn("Main price: 460.00 EGP", output.getvalue())
        self.assertIn("Seller: Sample seller", output.getvalue())

    def test_rejects_non_amazon_url(self):
        with contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit) as error:
                test_product.main(["https://example.com/dp/B0CPT6RBQY"])
        self.assertEqual(error.exception.code, 2)

    def test_simulated_alert_is_clearly_labeled(self):
        item = {
            "itemInfo": {"title": {"displayValue": "Sample headphones"}},
            "offersV2": {"listings": [{
                "price": {"money": {"amount": "460.00", "currency": "EGP"}},
                "merchantInfo": {"name": "Sample seller"},
            }]},
        }
        settings = Settings("www.amazon.eg", "test-tag", "", "", "3.2", "bot", "@channel",
                            source="scrape", scrape_currency="EGP")
        output = io.StringIO()
        with patch("test_product.Settings.from_file", return_value=settings), \
                patch("test_product.ScraperClient._get_item", return_value=item), \
                patch("test_product.capture_product_page", return_value={"price": "460.00",
                                                                         "seller": "Sample seller"}) as capture, \
                patch("test_product.TelegramNotifier.send_photo") as send:
            with contextlib.redirect_stdout(output):
                self.assertEqual(test_product.main(["--send-test-alert"]), 0)
        capture.assert_called_once()
        message = send.call_args.args[1]
        self.assertIn("اختبار فقط | انخفاض افتراضي على أمازون", message)
        self.assertIn("سعر سابق افتراضي: 506.00 جنيه", message)
        self.assertIn("السعر الآن: 460.00 جنيه", message)
        self.assertIn("فرق افتراضي فقط: 46.00 جنيه (9.1%)", message)
        self.assertIn("البائع: Sample seller", message)
        self.assertIn("https://www.amazon.eg/dp/B0CPT6RBQY?tag=test-tag", message)
        self.assertIn("السعر السابق افتراضي", message)
        self.assertIn("Photo test alert sent", output.getvalue())
        self.assertIn("prices.json was not changed", output.getvalue())


if __name__ == "__main__":
    unittest.main()
