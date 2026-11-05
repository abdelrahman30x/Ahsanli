import json
import tempfile
import threading
import unittest
from dataclasses import replace
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from unittest.mock import patch

from amazon_scraper import ScrapeError, parse_amount, parse_product_html
from request_policy import RequestGate
from test_request_policy import FakeClock
from watcher import ScraperClient, Settings, WatcherError, check_once


ASIN = "B012345678"
FIXTURES = Path(__file__).parent / "fixtures"


def fixture(name):
    return (FIXTURES / name).read_text(encoding="utf-8")


class PageServer(ThreadingHTTPServer):
    def __init__(self):
        super().__init__(("127.0.0.1", 0), PageHandler)
        self.pages = []
        self.requests = []


class PageHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.server.requests.append((self.path, self.headers.get("User-Agent"),
                                     self.headers.get("Proxy-Authorization")))
        status, body = self.server.pages.pop(0)
        encoded = body.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, _format, *args):
        pass


class FakeNotifier:
    def __init__(self):
        self.messages = []

    def send(self, message):
        self.messages.append(message)


class PhotoNotifier(FakeNotifier):
    def __init__(self):
        super().__init__()
        self.photos = []

    def send_photo(self, path, caption):
        self.photos.append((path.read_bytes(), caption))


class ScraperTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.settings = Settings("www.amazon.eg", "test-tag", "", "", "3.2", "bot", "123",
                                 source="scrape", scrape_currency="EGP")
        self.server = PageServer()
        thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.server.shutdown)

    def test_visible_buybox_price_beats_list_and_related_prices(self):
        result = parse_product_html(fixture("product_visible.html"), "EGP")
        self.assertEqual(result, {"price": "1299.00", "currency": "EGP",
                                  "title": "Sample Headphones", "seller": "ShopOne"})

    def test_marked_price_and_seller_boxes(self):
        result = parse_product_html(fixture("product_marked_boxes.html"), "EGP")
        self.assertEqual(result, {"price": "33814.94", "currency": "EGP",
                                  "title": "HONOR 400 Pro 5G, 12GB RAM, 512GB Storage",
                                  "seller": "Amazon.eg"})

    def test_structured_price_and_locale_formats(self):
        result = parse_product_html(fixture("product_jsonld.html"), "EGP")
        self.assertEqual(result, {"price": "1299.50", "currency": "EUR",
                                  "title": "Structured Product", "seller": "Amazon.eg"})
        self.assertEqual(parse_amount("EGP ١٬٢٩٩٫٠٠"), 1299)

    def test_block_or_missing_price_is_rejected(self):
        with self.assertRaisesRegex(ScrapeError, "block or CAPTCHA"):
            parse_product_html(fixture("robot_check.html"), "EGP")
        with self.assertRaisesRegex(ScrapeError, "No product price"):
            parse_product_html("<html><title>Missing</title></html>", "EGP")
        with self.assertRaisesRegex(ScrapeError, "No seller found"):
            parse_product_html('<div id="corePrice_feature_div"><span class="a-offscreen">EGP 10.00</span></div>',
                               "EGP")

    def test_injected_http_fetcher_keeps_affiliate_url_and_rejects_block_page(self):
        requested = []

        def fetch(url):
            requested.append(url)
            return fixture("product_visible.html")

        client = ScraperClient(self.settings, html_fetcher=fetch, sleep=lambda _: None)
        item = client.get_items([ASIN])[ASIN]
        self.assertEqual(requested, [f"https://www.amazon.eg/dp/{ASIN}"])
        self.assertIn("tag=test-tag", item["detailPageURL"])
        self.assertEqual(item["offersV2"]["listings"][0]["price"]["money"]["amount"], "1299.00")
        blocked = ScraperClient(self.settings, html_fetcher=lambda _: fixture("robot_check.html"))
        self.assertEqual(blocked.get_items([ASIN]), {})

    def test_mobile_batch_stops_on_blocks_and_retains_successful_prices(self):
        asins = [f"B{index:09d}" for index in range(10)]
        urls = self.root / "mobile_urls.txt"
        urls.write_text("".join(f"https://www.amazon.eg/dp/{asin}\n" for asin in asins), encoding="utf-8")
        state = self.root / "mobile_prices.json"
        clock = FakeClock()
        gate = RequestGate(6, 60, clock=clock.now, sleep=clock.sleep,
                           jitter=lambda _start, _end: 0.0)
        requested = []

        def fetch(url):
            requested.append((url, clock.now()))
            return fixture("product_visible.html" if len(requested) == 1 else "robot_check.html")

        client = ScraperClient(self.settings, html_fetcher=fetch, request_gate=gate, sleep=clock.sleep)
        notifier = FakeNotifier()
        self.assertFalse(check_once(self.settings, urls, state, client, notifier))
        self.assertEqual(len(requested), 4)
        self.assertEqual([time for _, time in requested], [0.0, 10.0, 20.0, 30.0])
        self.assertEqual(json.loads(state.read_text(encoding="utf-8"))["products"][asins[0]]["price"], "1299.00")
        self.assertEqual(notifier.messages, [])

    def test_direct_scrape_reuses_price_history_and_alerts(self):
        urls = self.root / "urls.txt"
        urls.write_text(f"https://www.amazon.eg/dp/{ASIN}\n", encoding="utf-8")
        state = self.root / "prices.json"
        notifier = PhotoNotifier()
        self.server.pages = [(200, fixture("product_visible.html").replace("EGP 1,299.00", "EGP 1,499.00")),
                             (200, fixture("product_drop.html")),
                             (200, fixture("robot_check.html"))]
        page_url = f"http://127.0.0.1:{self.server.server_port}/dp/{{asin}}"
        client = ScraperClient(self.settings, page_url_template=page_url, sleep=lambda _: None)
        def capture(_url, output, _currency):
            output.write_bytes(b"screenshot")
            return {"price": "1199.00", "seller": "ShopTwo"}

        with patch("capture_product.capture_product_page", side_effect=capture):
            self.assertTrue(check_once(self.settings, urls, state, client, notifier))
            self.assertTrue(check_once(self.settings, urls, state, client, notifier))
            self.assertFalse(check_once(self.settings, urls, state, client, notifier))
        self.assertEqual(notifier.messages, [])
        self.assertEqual(len(notifier.photos), 1)
        self.assertIn("سعر العرض: 1,199.00 جنيه", notifier.photos[0][1])
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 20%", notifier.photos[0][1])
        self.assertNotIn("1,499.00 جنيه", notifier.photos[0][1])
        self.assertIn("البائع: ShopTwo", notifier.photos[0][1])
        self.assertIn("البائع السابق: ShopOne", notifier.photos[0][1])
        self.assertIn("tag=test-tag", notifier.photos[0][1])
        saved = json.loads(state.read_text(encoding="utf-8"))["products"][ASIN]
        self.assertEqual(saved["price"], "1199.00")
        self.assertEqual(saved["seller"], "ShopTwo")
        self.assertEqual(self.server.requests[0][1], "Agent/AmazonPriceWatcher")

    def test_drop_sends_matching_screenshot_with_caption(self):
        urls = self.root / "urls.txt"
        urls.write_text(f"https://www.amazon.eg/dp/{ASIN}\n", encoding="utf-8")
        state = self.root / "prices.json"
        self.server.pages = [(200, fixture("product_visible.html").replace("EGP 1,299.00", "EGP 1,499.00")),
                             (200, fixture("product_drop.html"))]
        client = ScraperClient(self.settings,
                               page_url_template=f"http://127.0.0.1:{self.server.server_port}/dp/{{asin}}")
        notifier = PhotoNotifier()

        def capture(_url, output, _currency):
            output.write_bytes(b"screenshot")
            return {"price": "1199.00", "seller": "ShopTwo",
                    "store_discount_percent": "25%", "stock_count": 3}

        with patch("capture_product.capture_product_page", side_effect=capture):
            self.assertTrue(check_once(self.settings, urls, state, client, notifier))
            self.assertTrue(check_once(self.settings, urls, state, client, notifier))
        self.assertEqual(notifier.messages, [])
        self.assertEqual(len(notifier.photos), 1)
        self.assertEqual(notifier.photos[0][0], b"screenshot")
        self.assertIn("الخصم المعروض على صفحة أمازون: 25%", notifier.photos[0][1])
        self.assertIn("آخر 3 قطع", notifier.photos[0][1])
        self.assertNotIn("آخر سعر رصدناه بنسبة", notifier.photos[0][1])
        self.assertEqual(json.loads(state.read_text(encoding="utf-8"))["products"][ASIN]["pending"], [])

    def test_mismatched_screenshot_is_held_after_three_checks_without_text(self):
        urls = self.root / "urls.txt"
        urls.write_text(f"https://www.amazon.eg/dp/{ASIN}\n", encoding="utf-8")
        state = self.root / "prices.json"
        self.server.pages = [(200, fixture("product_visible.html").replace("EGP 1,299.00", "EGP 1,499.00")),
                             (200, fixture("product_drop.html")),
                             (200, fixture("product_drop.html")),
                             (200, fixture("product_drop.html"))]
        client = ScraperClient(self.settings,
                               page_url_template=f"http://127.0.0.1:{self.server.server_port}/dp/{{asin}}")
        notifier = PhotoNotifier()
        with patch("capture_product.capture_product_page", return_value={"price": "999.00",
                                                                         "seller": "ShopTwo"}):
            self.assertTrue(check_once(self.settings, urls, state, client, notifier))
            for _ in range(3):
                self.assertFalse(check_once(self.settings, urls, state, client, notifier))
        self.assertEqual(notifier.photos, [])
        self.assertEqual(notifier.messages, [])
        saved = json.loads(state.read_text(encoding="utf-8"))
        self.assertEqual(saved["products"][ASIN]["pending"], [])
        self.assertEqual(len(saved["held_alerts"]), 1)
        self.assertEqual(saved["held_alerts"][0]["capture_attempts"], 3)

    def test_configured_proxy_is_used_and_direct_mode_ignores_it(self):
        proxy = f"http://user:pass@127.0.0.1:{self.server.server_port}"
        settings = replace(self.settings, proxy_url=proxy)
        self.server.pages = [(200, fixture("product_visible.html"))]
        client = ScraperClient(settings, page_url_template="http://example.test/dp/{asin}", sleep=lambda _: None)
        result = client.get_items([ASIN])
        self.assertEqual(result[ASIN]["offersV2"]["listings"][0]["price"]["money"]["amount"], "1299.00")
        self.assertEqual(self.server.requests[0][0], f"http://example.test/dp/{ASIN}")
        self.assertTrue(self.server.requests[0][2].startswith("Basic "))
        direct = ScraperClient(replace(settings, proxy_url=""),
                               page_url_template=f"http://127.0.0.1:{self.server.server_port}/dp/{{asin}}",
                               sleep=lambda _: None)
        self.server.pages = [(200, fixture("product_drop.html"))]
        self.assertIn(ASIN, direct.get_items([ASIN]))
        self.assertEqual(self.server.requests[1][0], f"/dp/{ASIN}")
        self.assertIsNone(self.server.requests[1][2])

    def test_scrape_settings_do_not_require_api_keys(self):
        path = self.root / "settings.json"
        path.write_text(json.dumps({
            "source": "scrape", "marketplace": "www.amazon.eg", "partner_tag": "test-tag",
            "scrape_currency": "EGP", "telegram_bot_token": "bot", "telegram_chat_id": "123",
            "proxy_url": "",
        }), encoding="utf-8")
        settings = Settings.from_file(path)
        self.assertEqual(settings.source, "scrape")
        self.assertEqual(settings.credential_id, "")
        data = json.loads(path.read_text(encoding="utf-8"))
        data["proxy_url"] = "socks5://localhost:1234"
        path.write_text(json.dumps(data), encoding="utf-8")
        with self.assertRaisesRegex(WatcherError, "proxy_url"):
            Settings.from_file(path)

    def test_switching_from_api_price_starts_new_scrape_baseline(self):
        urls = self.root / "urls.txt"
        urls.write_text(f"https://www.amazon.eg/dp/{ASIN}\n", encoding="utf-8")
        state = self.root / "prices.json"
        self.server.pages = [(200, fixture("product_drop.html"))]
        client = ScraperClient(self.settings,
                               page_url_template=f"http://127.0.0.1:{self.server.server_port}/dp/{{asin}}")
        notifier = FakeNotifier()
        state.write_text(json.dumps({"version": 1, "products": {ASIN: {
            "price": "1500.00", "currency": "EGP", "source": "api",
            "observed_at": datetime.now(timezone.utc).isoformat(), "pending": [],
        }}}), encoding="utf-8")
        self.assertTrue(check_once(self.settings, urls, state, client, notifier))
        self.assertEqual(notifier.messages, [])
        self.assertEqual(json.loads(state.read_text(encoding="utf-8"))["products"][ASIN]["source"], "scrape")


if __name__ == "__main__":
    unittest.main()
