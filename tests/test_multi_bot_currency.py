"""Tests for multi-bot Telegram routing and per-site currency localization."""

from __future__ import annotations

import unittest
from decimal import Decimal
from unittest import mock

from config import SiteConfig, TrackerConfig
from db.database import TrackerDB
from post_format import _money, format_price_post
from price_checker import PriceChecker


class TestMultiBotCurrency(unittest.TestCase):
    def test_money_formatting_for_various_currencies(self):
        # Egyptian Pound
        self.assertEqual(_money(Decimal("1250.50"), "EGP"), "1,250.50 جنيه")
        self.assertEqual(_money(Decimal("45.00"), "egp"), "45.00 جنيه")

        # Saudi Riyal
        self.assertEqual(_money(Decimal("320.00"), "SAR"), "320.00 ر.س")

        # UAE Dirham
        self.assertEqual(_money(Decimal("199.99"), "AED"), "199.99 د.إ")

        # Kuwaiti Dinar
        self.assertEqual(_money(Decimal("15.50"), "KWD"), "15.50 د.ك")

        # US Dollar
        self.assertEqual(_money(Decimal("29.99"), "USD"), "29.99 USD")

    def test_config_site_custom_bot_and_currency(self):
        cfg_dict = {
            "telegram_bot_token": "global_bot_token",
            "telegram_chat_id": "@global_channel",
            "sites": [
                {
                    "code": "amazon_eg",
                    "marketplace": "www.amazon.eg",
                    "currency": "EGP",
                    "store_name": "أمازون مصر",
                    "telegram_bot_token": "eg_bot_token",
                    "telegram_chat_id": "@eg_deals",
                },
                {
                    "code": "amazon_com",
                    "marketplace": "www.amazon.com",
                    "currency": "USD",
                    "store_name": "أمازون أمريكا",
                },
            ],
        }
        config = TrackerConfig.from_dict(cfg_dict)
        self.assertEqual(len(config.sites), 2)
        self.assertEqual(config.min_alert_drop_percent, 10.0)

        eg_site = config.sites[0]
        self.assertEqual(eg_site.code, "amazon_eg")
        self.assertEqual(eg_site.currency, "EGP")
        self.assertEqual(eg_site.store_name, "أمازون مصر")
        self.assertEqual(eg_site.telegram_bot_token, "eg_bot_token")
        self.assertEqual(eg_site.telegram_chat_id, "@eg_deals")

        com_site = config.sites[1]
        self.assertEqual(com_site.code, "amazon_com")
        self.assertEqual(com_site.currency, "USD")
        self.assertEqual(com_site.store_name, "أمازون أمريكا")
        self.assertEqual(com_site.telegram_bot_token, "")

        cfg_dict["min_alert_drop_percent"] = 15
        self.assertEqual(TrackerConfig.from_dict(cfg_dict).min_alert_drop_percent, 15.0)
        self.assertEqual(TrackerConfig.from_dict(cfg_dict).to_dict()["min_alert_drop_percent"], 15.0)
        cfg_dict["min_alert_drop_percent"] = -1
        with self.assertRaises(ValueError):
            TrackerConfig.from_dict(cfg_dict)

    def test_database_site_metadata_persistence(self):
        db = TrackerDB(":memory:")
        site_id = db.get_or_create_site(
            code="amazon_eg",
            marketplace="www.amazon.eg",
            partner_tag="egypt-21",
            currency="EGP",
            store_name="أمازون مصر",
            telegram_bot_token="eg_token_123",
            telegram_chat_id="@egypt_deals",
        )

        site = db.get_site(site_id)
        self.assertIsNotNone(site)
        self.assertEqual(site["code"], "amazon_eg")
        self.assertEqual(site["currency"], "EGP")
        self.assertEqual(site["store_name"], "أمازون مصر")
        self.assertEqual(site["telegram_bot_token"], "eg_token_123")
        self.assertEqual(site["telegram_chat_id"], "@egypt_deals")

    def test_alert_routing_to_specific_bot_and_egp_currency(self):
        db = TrackerDB(":memory:")
        site_id = db.get_or_create_site(
            code="amazon_eg",
            marketplace="www.amazon.eg",
            partner_tag="egypt-21",
            currency="EGP",
            store_name="أمازون مصر",
            telegram_bot_token="bot_token_eg",
            telegram_chat_id="@eg_channel",
        )

        # Upsert product and record price drop
        db.upsert_product(
            asin="B001122334",
            site_id=site_id,
            title="سماعة سلكية أصلية",
            price="1000.00",
            currency="EGP",
            seller="Amazon Egypt",
            affiliate_url="https://www.amazon.eg/dp/B001122334?tag=egypt-21",
        )

        alert = db.record_price(
            asin="B001122334",
            site_id=site_id,
            price="800.00",
            currency="EGP",
            seller="Amazon Egypt",
        )
        self.assertIsNotNone(alert)

        # Set up PriceChecker with mock default notifier
        default_notifier = mock.Mock()
        checker = PriceChecker(db=db, notifier=default_notifier)

        def capture(_url, output, _currency):
            output.write_bytes(b"\x89PNG\r\n\x1a\npage")
            return {"price": "800.00", "seller": "Amazon Egypt"}
        with mock.patch("watcher.TelegramNotifier.send_photo") as mock_site_send, \
                mock.patch("capture_product.capture_product_page", side_effect=capture):
            sent = checker.dispatch_pending_alerts(site_id=site_id)
            self.assertEqual(sent, 1)

            # Assert specific site bot received the photo, not the default notifier.
            default_notifier.send_photo.assert_not_called()
            mock_site_send.assert_called_once()

            # Verify the message content contains EGP formatted as 'جنيه' and store as 'أمازون مصر'
            sent_message = mock_site_send.call_args[0][1]
            self.assertIn("أمازون مصر", sent_message)
            self.assertIn("800.00 جنيه", sent_message)
            self.assertIn("أقل من آخر سعر رصدناه بنسبة 20%", sent_message)
            self.assertNotIn("1,000.00 جنيه", sent_message)

    def test_alert_fallback_to_global_bot_when_site_has_no_bot(self):
        db = TrackerDB(":memory:")
        site_id = db.get_or_create_site(
            code="amazon_com",
            marketplace="www.amazon.com",
            partner_tag="global-21",
            currency="USD",
            store_name="أمازون أمريكا",
            telegram_bot_token="",
            telegram_chat_id="",
        )

        db.upsert_product(
            asin="B08N5WRWNW",
            site_id=site_id,
            title="Smart Speaker",
            price="50.00",
            currency="USD",
        )
        db.record_price(
            asin="B08N5WRWNW",
            site_id=site_id,
            price="35.00",
            currency="USD",
        )

        default_notifier = mock.Mock(spec=["send_photo"])
        checker = PriceChecker(db=db, notifier=default_notifier)

        def capture(_url, output, _currency):
            output.write_bytes(b"\x89PNG\r\n\x1a\npage")
            return {"price": "35.00", "seller": "Seller"}
        with mock.patch("capture_product.capture_product_page", side_effect=capture):
            sent = checker.dispatch_pending_alerts(site_id=site_id)
        self.assertEqual(sent, 1)
        default_notifier.send_photo.assert_called_once()
        msg = default_notifier.send_photo.call_args[0][1]
        self.assertIn("أمازون أمريكا", msg)
        self.assertIn("35.00 USD", msg)


if __name__ == "__main__":
    unittest.main()
