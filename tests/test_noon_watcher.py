import json
import tempfile
import unittest
from pathlib import Path

from noon_capture import NoonCaptureError, parse_price, parse_seller
from noon_http import fetch_noon_offer
from noon_watcher import (NoonSettings, check_once, parse_noon_url, read_noon_products,
                          send_test_drop)
from watcher import WatcherError


URL = ("https://www.noon.com/egypt-en/p9-headphones/"
       "ZC1966B64128594CBA01EZ/p/?o=zc1966b64128594cba01ez-1")


class FakeNoonFetcher:
    def __init__(self):
        self.price = "100.00"
        self.seller = "First Seller"
        self.capture_price = None
        self.error = None
        self.capture_calls = 0

    def __call__(self, url, *, currency, proxy_url, output=None):
        if self.error:
            raise self.error
        if output is not None:
            self.capture_calls += 1
            output.write_bytes(b"\x89PNG\r\n\x1a\nfixture")
        price = self.capture_price if output is not None and self.capture_price else self.price
        return {"title": "P9 Headphones", "price": price,
                "currency": currency, "seller": self.seller, "url": url}


class FakeNotifier:
    def __init__(self):
        self.photos = []
        self.messages = []

    def send_photo(self, path, caption):
        self.photos.append((path.read_bytes(), caption))

    def send(self, message):
        self.messages.append(message)


class NoonWatcherTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.urls = self.root / "noon_urls.txt"
        self.urls.write_text(URL + "\n", encoding="utf-8")
        self.state = self.root / "noon_prices.json"
        self.settings = NoonSettings("bot", "@channel")
        self.fetch = FakeNoonFetcher()
        self.notifier = FakeNotifier()

    def check(self, changes=None):
        options = {} if changes is None else {"changes": changes}
        return check_once(self.settings, self.urls, self.state,
                          fetch=self.fetch, capture=self.fetch,
                          notifier=self.notifier, **options)

    def test_noon_url_and_optional_affiliate_link(self):
        key = parse_noon_url(URL)
        self.assertEqual(key, "noon:ZC1966B64128594CBA01EZ:zc1966b64128594cba01ez-1")
        self.urls.write_text(URL + "\thttps://track.example/noon-123\n" + URL + "\n",
                             encoding="utf-8")
        products = read_noon_products(self.urls)
        self.assertEqual(len(products), 1)
        self.assertEqual(products[0].post_url, "https://track.example/noon-123")
        for bad in ("https://example.com/egypt-en/p9/N53330542A/p/",
                    "http://www.noon.com/egypt-en/p9/N53330542A/p/",
                    "https://www.noon.com/uae-en/p9/N53330542A/p/"):
            with self.assertRaises(WatcherError):
                parse_noon_url(bad)

    def test_default_periodic_fetch_is_http_only(self):
        self.assertIs(check_once.__kwdefaults__["fetch"], fetch_noon_offer)

    def test_price_and_seller_parsers(self):
        self.assertEqual(parse_price("EGP 1,310.50"), 1310.50)
        self.assertEqual(parse_price("١٬٣١٠٫٥٠ جنيه"), 1310.50)
        with self.assertRaises(NoonCaptureError):
            parse_price("EGP 310 EGP 499")
        self.assertEqual(parse_seller("Sold by\nAin Store\n4.5 59% Positive"), "Ain Store")
        self.assertEqual(parse_seller("Sold by Original Stores 4.9 95% Positive"),
                         "Original Stores")

    def test_initial_baseline_then_drop_and_increase_with_matching_photo(self):
        self.assertTrue(self.check())
        self.assertEqual(self.notifier.photos, [])
        self.assertEqual(self.fetch.capture_calls, 0)
        self.fetch.price = "90.00"
        self.assertTrue(self.check())
        self.assertEqual(self.fetch.capture_calls, 1)
        self.fetch.price = "110.00"
        self.assertTrue(self.check())
        self.assertEqual(self.fetch.capture_calls, 1)
        self.assertEqual(len(self.notifier.photos), 1)
        self.assertIn("السعر نزل على نون", self.notifier.photos[0][1])
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 10%", self.notifier.photos[0][1])
        self.assertEqual(self.notifier.messages, [])
        self.assertEqual(json.loads(self.state.read_text(encoding="utf-8"))["products"]
                         [parse_noon_url(URL)]["price"], "110.00")

    def test_default_mode_compares_with_last_observed_price(self):
        self.assertTrue(self.check())
        self.fetch.price = "120.00"
        self.assertTrue(self.check())
        self.assertEqual(self.notifier.photos, [])
        self.assertTrue(self.check())
        self.assertEqual(self.notifier.photos, [])
        self.fetch.price = "115.00"
        self.assertTrue(self.check())
        self.assertEqual(len(self.notifier.photos), 0)
        self.assertEqual(self.fetch.capture_calls, 0)
        self.fetch.price = "100.00"
        self.assertTrue(self.check())
        self.assertEqual(len(self.notifier.photos), 1)
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 13%", self.notifier.photos[0][1])

    def test_any_mode_remains_opt_in(self):
        self.assertTrue(self.check(changes="any"))
        self.fetch.price = "120.00"
        self.assertTrue(self.check(changes="any"))
        self.assertEqual(len(self.notifier.photos), 1)
        self.assertIn("تحديث سعر على نون", self.notifier.photos[0][1])
        self.assertIn("أعلى من آخر سعر رصدناه بنسبة 20%", self.notifier.photos[0][1])
        self.assertNotIn("التوفير", self.notifier.photos[0][1])

    def test_capture_mismatch_falls_back_to_text(self):
        self.assertTrue(self.check())
        self.fetch.price = "90.00"
        self.fetch.capture_price = "89.00"
        self.assertTrue(self.check())
        self.assertEqual(self.notifier.photos, [])
        self.assertEqual(len(self.notifier.messages), 1)
        self.assertIn("سعر العرض: 90.00 جنيه", self.notifier.messages[0])

    def test_coupon_is_labeled_unverified_not_used_as_tracking(self):
        self.settings = NoonSettings("bot", "@channel", coupon_code="HG82")
        self.assertTrue(self.check())
        self.fetch.price = "90.00"
        self.assertTrue(self.check())
        caption = self.notifier.photos[0][1]
        self.assertIn("كود خصم غير متحقق من صلاحيته: HG82", caption)
        self.assertNotIn("tag=HG82", caption)

    def test_simulated_drop_sends_matching_photo_without_saving_history(self):
        product = read_noon_products(self.urls)[0]
        alert = send_test_drop(self.settings, product, fetch=self.fetch,
                               notifier=self.notifier)
        self.assertEqual(alert["old_price"], "110.00")
        self.assertEqual(len(self.notifier.photos), 1)
        self.assertEqual(self.notifier.messages, [])
        caption = self.notifier.photos[0][1]
        self.assertIn("اختبار فقط | انخفاض افتراضي على نون", caption)
        self.assertIn("سعر سابق افتراضي: 110.00 جنيه", caption)
        self.assertIn("السعر الآن: 100.00 جنيه", caption)
        self.assertIn("فرق افتراضي فقط: 10.00 جنيه (9.1%)", caption)
        self.assertIn("السعر السابق افتراضي", caption)
        self.assertFalse(self.state.exists())

    def test_simulated_drop_rejects_mismatched_photo(self):
        product = read_noon_products(self.urls)[0]
        self.fetch.capture_price = "99.00"
        with self.assertRaisesRegex(WatcherError, "photo could not be verified"):
            send_test_drop(self.settings, product, fetch=self.fetch,
                           notifier=self.notifier)
        self.assertEqual(self.notifier.photos, [])
        self.assertEqual(self.notifier.messages, [])
        self.assertFalse(self.state.exists())

    def test_failed_scrape_keeps_previous_price(self):
        self.assertTrue(self.check())
        self.fetch.error = NoonCaptureError("blocked")
        self.assertFalse(self.check())
        record = json.loads(self.state.read_text(encoding="utf-8"))["products"][parse_noon_url(URL)]
        self.assertEqual(record["price"], "100.00")
        self.assertEqual(self.notifier.photos, [])
        self.assertEqual(self.fetch.capture_calls, 0)


if __name__ == "__main__":
    unittest.main()
