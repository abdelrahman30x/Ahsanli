import json
import os
import tempfile
import threading
import unittest
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from unittest.mock import patch

from watcher import AmazonClient, Settings, TelegramNotifier, WatcherError, check_once, get_telegram_chat_ids, read_asins


ASIN_A = "B012345678"
ASIN_B = "B087654321"


def item(asin, amount=None, currency="EGP"):
    result = {
        "asin": asin,
        "detailPageURL": f"https://www.amazon.eg/dp/{asin}?tag=test-tag",
        "itemInfo": {"title": {"displayValue": f"Product {asin}"}},
        "offersV2": {"listings": []},
    }
    if amount is not None:
        result["offersV2"]["listings"] = [{
            "condition": {"value": "New"},
            "isBuyBoxWinner": True,
            "merchantInfo": {"name": "Amazon.eg"},
            "price": {"money": {"amount": amount, "currency": currency}},
        }]
    return result


class FakeServices(ThreadingHTTPServer):
    def __init__(self):
        super().__init__(("127.0.0.1", 0), FakeHandler)
        self.snapshots = []
        self.messages = []
        self.photos = []
        self.token_calls = 0
        self.api_calls = 0
        self.telegram_failures = 0
        self.request_bodies = []


class FakeHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/bottest-bot/getUpdates?timeout=0":
            self.reply(200, {"ok": True, "result": [{"message": {"chat": {"id": 12345}}}]})
        else:
            self.reply(404, {})

    def do_POST(self):
        length = int(self.headers["Content-Length"])
        if self.path == "/bottest-bot/sendPhoto":
            self.server.photos.append((self.headers["Content-Type"], self.rfile.read(length)))
            self.reply(200, {"ok": True})
            return
        body = json.loads(self.rfile.read(length))
        self.server.request_bodies.append((self.path, body))
        if self.path == "/token":
            self.server.token_calls += 1
            self.reply(200, {"access_token": "test-token", "expires_in": 3600})
        elif self.path == "/catalog/v1/getItems":
            self.server.api_calls += 1
            if self.headers.get("Authorization") != "Bearer test-token":
                self.reply(401, {"error": "bad token"})
                return
            snapshot = self.server.snapshots.pop(0)
            self.reply(200, {"itemsResult": {"items": snapshot}})
        elif self.path == "/bottest-bot/sendMessage":
            if self.server.telegram_failures:
                self.server.telegram_failures -= 1
                self.reply(500, {"ok": False})
            else:
                self.server.messages.append(body)
                self.reply(200, {"ok": True})
        else:
            self.reply(404, {})

    def reply(self, status, data):
        encoded = json.dumps(data).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, _format, *args):
        pass


class WatcherTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.urls = self.root / "urls.txt"
        self.urls.write_text(f"https://www.amazon.eg/dp/{ASIN_A}?tag=old\n", encoding="utf-8")
        self.state = self.root / "prices.json"
        self.settings = Settings("www.amazon.eg", "test-tag", "id", "secret", "3.2", "test-bot", "12345")
        self.server = FakeServices()
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.server.shutdown)
        base = f"http://127.0.0.1:{self.server.server_port}"
        self.client = AmazonClient(self.settings, token_url=f"{base}/token",
                                   api_url=f"{base}/catalog/v1/getItems", sleep=lambda _: None)
        self.notifier = TelegramNotifier(self.settings, base_url=base)

    def check(self):
        return check_once(self.settings, self.urls, self.state, self.client, self.notifier)

    def test_telegram_chat_lookup(self):
        base = f"http://127.0.0.1:{self.server.server_port}"
        self.assertEqual(get_telegram_chat_ids("test-bot", base_url=base), ["12345"])
        with self.assertRaisesRegex(WatcherError, "TELEGRAM_BOT_TOKEN"):
            get_telegram_chat_ids("")

    def test_telegram_photo_upload(self):
        path = self.root / "product.png"
        photo = b"\x89PNG\r\n\x1a\nphoto-bytes"
        path.write_bytes(photo)
        self.notifier.send_photo(path, "Price: 460 EGP\nSeller: ShopOne")
        self.assertEqual(len(self.server.photos), 1)
        content_type, body = self.server.photos[0]
        self.assertIn("multipart/form-data; boundary=", content_type)
        self.assertIn(b'name="chat_id"', body)
        self.assertIn(b"12345", body)
        self.assertIn(b'name="caption"', body)
        self.assertIn(b"Seller: ShopOne", body)
        self.assertIn(b'name="photo"; filename="product.png"', body)
        self.assertIn(photo, body)

    def test_telegram_environment_overrides_settings_file(self):
        path = self.root / "settings.json"
        path.write_text(json.dumps({
            "source": "scrape", "marketplace": "www.amazon.eg", "partner_tag": "test-tag",
            "scrape_currency": "EGP", "telegram_bot_token": "", "telegram_chat_id": "",
        }), encoding="utf-8")
        with patch.dict(os.environ, {"TELEGRAM_BOT_TOKEN": "new-token", "TELEGRAM_CHAT_ID": "12345"}):
            settings = Settings.from_file(path)
        self.assertEqual(settings.telegram_bot_token, "new-token")
        self.assertEqual(settings.telegram_chat_id, "12345")

    def test_first_price_then_drops_only_with_persisted_baseline(self):
        self.urls.write_text(
            f"https://www.amazon.eg/dp/{ASIN_A}\n"
            f"https://www.amazon.eg/gp/product/{ASIN_B}?tag=other\n"
            f"https://www.amazon.eg/dp/{ASIN_A}\n", encoding="utf-8")
        self.server.snapshots = [
            [item(ASIN_A, "100.00"), item(ASIN_B, "200.00")],
            [item(ASIN_A, "90.00"), item(ASIN_B, "200.00")],
            [item(ASIN_A, "95.00"), item(ASIN_B, "180.00")],
            [item(ASIN_A, "95.00"), item(ASIN_B, "180.00")],
        ]
        for _ in range(4):
            self.assertTrue(self.check())
        self.assertEqual(len(self.server.messages), 2)
        self.assertIn("سعر العرض: 90.00 جنيه", self.server.messages[0]["text"])
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 10%", self.server.messages[0]["text"])
        self.assertNotIn("100.00 جنيه", self.server.messages[0]["text"])
        self.assertIn("البائع: Amazon.eg", self.server.messages[0]["text"])
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 10%", self.server.messages[1]["text"])
        self.assertIn("tag=test-tag", self.server.messages[0]["text"])
        saved = json.loads(self.state.read_text(encoding="utf-8"))
        self.assertEqual(saved["products"][ASIN_A]["price"], "95.00")
        self.assertEqual(saved["products"][ASIN_B]["price"], "180.00")
        self.assertEqual(self.server.token_calls, 1)
        self.assertEqual([len(body["itemIds"]) for path, body in self.server.request_bodies
                          if path.endswith("getItems")], [2, 2, 2, 2])

    def test_failed_telegram_send_is_queued_and_retried(self):
        self.server.snapshots = [[item(ASIN_A, "100.00")], [item(ASIN_A, "80.00")], [item(ASIN_A, "90.00")]]
        self.assertTrue(self.check())
        self.server.telegram_failures = 3
        self.assertFalse(self.check())
        saved = json.loads(self.state.read_text(encoding="utf-8"))
        self.assertEqual(saved["products"][ASIN_A]["price"], "80.00")
        self.assertEqual(len(saved["products"][ASIN_A]["pending"]), 1)
        self.assertTrue(self.check())
        self.assertEqual(len(self.server.messages), 1)
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 20%", self.server.messages[0]["text"])
        self.assertEqual(json.loads(self.state.read_text(encoding="utf-8"))["products"][ASIN_A]["pending"], [])

    def test_missing_price_does_not_erase_previous_price(self):
        self.server.snapshots = [[item(ASIN_A, "100.00")], [item(ASIN_A)], [item(ASIN_A, "90.00")]]
        self.assertTrue(self.check())
        self.assertTrue(self.check())
        self.assertTrue(self.check())
        self.assertEqual(len(self.server.messages), 1)
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 10%", self.server.messages[0]["text"])

    def test_small_drop_updates_baseline_without_publishing(self):
        self.server.snapshots = [[item(ASIN_A, "3500.00")], [item(ASIN_A, "3485.00")]]
        self.assertTrue(self.check())
        self.assertTrue(self.check())
        self.assertEqual(self.server.messages, [])
        saved = json.loads(self.state.read_text(encoding="utf-8"))
        self.assertEqual(saved["products"][ASIN_A]["price"], "3485.00")
        self.assertEqual(saved["products"][ASIN_A]["pending"], [])

    def test_currency_change_starts_new_baseline(self):
        self.server.snapshots = [[item(ASIN_A, "100.00", "EGP")], [item(ASIN_A, "1.00", "USD")]]
        self.assertTrue(self.check())
        self.assertTrue(self.check())
        self.assertEqual(self.server.messages, [])

    def test_expired_price_starts_new_baseline(self):
        self.server.snapshots = [[item(ASIN_A, "100.00")], [item(ASIN_A, "90.00")]]
        self.assertTrue(self.check())
        saved = json.loads(self.state.read_text(encoding="utf-8"))
        saved["products"][ASIN_A]["observed_at"] = (
            datetime.now(timezone.utc) - timedelta(hours=25)).isoformat()
        self.state.write_text(json.dumps(saved), encoding="utf-8")
        self.assertTrue(self.check())
        self.assertEqual(self.server.messages, [])
        self.assertEqual(json.loads(self.state.read_text(encoding="utf-8"))["products"][ASIN_A]["price"], "90.00")

    def test_out_of_stock_and_restock_alert(self):
        self.server.snapshots = [[item(ASIN_A, "100.00")]]
        self.assertTrue(self.check())
        self.assertEqual(len(self.server.messages), 0)

        # Goes out of stock
        self.server.snapshots = [[{"asin": ASIN_A, "out_of_stock": True}]]
        self.assertTrue(self.check())
        saved = json.loads(self.state.read_text(encoding="utf-8"))
        self.assertFalse(saved["products"][ASIN_A]["in_stock"])

        # Restocks even at a higher price
        self.server.snapshots = [[item(ASIN_A, "110.00")]]
        self.assertTrue(self.check())
        self.assertEqual(len(self.server.messages), 1)
        self.assertIn("رجع اتوفر", self.server.messages[0]["text"])

    def test_url_validation(self):
        self.urls.write_text(f"https://www.amazon.com/dp/{ASIN_A}\n", encoding="utf-8")
        with self.assertRaisesRegex(WatcherError, "expected an HTTPS product URL"):
            read_asins(self.urls, "www.amazon.eg")
        self.urls.write_text("https://amzn.to/abc123\n", encoding="utf-8")
        with self.assertRaises(WatcherError):
            read_asins(self.urls, "www.amazon.eg")


if __name__ == "__main__":
    unittest.main()
