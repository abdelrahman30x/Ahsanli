import threading
import time
import unittest
import importlib.util

from capture_product import (
    BLOCKED_RESOURCE_TYPES,
    BLOCKED_URL_PATTERNS,
    CaptureError,
    CaptureQueue,
    asin_from_capture_url,
    cookie_notice_visible,
    dismiss_cookie_notice,
    element_unobstructed,
    parse_low_stock_count,
    parse_store_discount,
    product_clip,
    wait_for_offer_regions,
)


class ProductClipTests(unittest.TestCase):
    def test_capture_cli_accepts_only_amazon_product_urls(self):
        self.assertEqual(asin_from_capture_url("https://www.amazon.eg/dp/B0HJWKCLYR"), "B0HJWKCLYR")
        with self.assertRaises(ValueError):
            asin_from_capture_url("https://example.com/dp/B0HJWKCLYR")

    def test_low_stock_requires_explicit_limited_count(self):
        self.assertEqual(parse_low_stock_count("Only 3 left in stock - order soon."), 3)
        self.assertEqual(parse_low_stock_count("Only 2 left in stock - order soon."), 2)
        self.assertIsNone(parse_low_stock_count("In Stock"))
        self.assertIsNone(parse_low_stock_count("Only 12 left in stock"))
        self.assertIsNone(parse_low_stock_count("Only 0 left in stock"))

    def test_store_discount_only_accepts_price_badge_percentage(self):
        self.assertEqual(parse_store_discount("-21%"), "21%")
        self.assertEqual(parse_store_discount("−21.5 %"), "21.5%")
        self.assertIsNone(parse_store_discount("Save 5% on 5 items"))
        self.assertIsNone(parse_store_discount("0%"))
        self.assertIsNone(parse_store_discount("120%"))

    def test_crops_to_image_title_and_buybox(self):
        rectangles = [
            {"x": 18, "y": 206, "width": 512, "height": 488},
            {"x": 551, "y": 208, "width": 596, "height": 91},
            {"x": 1178, "y": 377, "width": 244, "height": 482},
        ]
        self.assertEqual(product_clip(rectangles, {"width": 1440, "height": 900}), {
            "x": 0, "y": 194, "width": 1440, "height": 681,
        })

    def test_clamps_crop_to_viewport(self):
        rectangles = [{"x": -8, "y": -4, "width": 180, "height": 160}]
        self.assertEqual(product_clip(rectangles, {"width": 100, "height": 100}), {
            "x": 0, "y": 0, "width": 100, "height": 100,
        })

    def test_rejects_product_outside_viewport(self):
        rectangles = [{"x": 200, "y": 200, "width": 20, "height": 20}]
        with self.assertRaises(CaptureError):
            product_clip(rectangles, {"width": 100, "height": 100})

    def test_blocked_resources_and_patterns_defined(self):
        self.assertIn("media", BLOCKED_RESOURCE_TYPES)
        self.assertIn("font", BLOCKED_RESOURCE_TYPES)
        self.assertTrue(any("amazon-adsystem" in p for p in BLOCKED_URL_PATTERNS))
        self.assertTrue(any("fls-na" in p for p in BLOCKED_URL_PATTERNS))
        self.assertTrue(any(".mp4" in p for p in BLOCKED_URL_PATTERNS))

    def test_capture_queue_sequential_execution(self):
        order = []

        def worker(idx):
            with CaptureQueue.acquire(timeout=5.0):
                order.append(f"start_{idx}")
                time.sleep(0.05)
                order.append(f"end_{idx}")

        t1 = threading.Thread(target=worker, args=(1,))
        t2 = threading.Thread(target=worker, args=(2,))
        t1.start()
        t2.start()
        t1.join()
        t2.join()

        self.assertEqual(len(order), 4)
        # Ensure non-overlapping execution (start_A, end_A, start_B, end_B)
        self.assertIn(order, (
            ["start_1", "end_1", "start_2", "end_2"],
            ["start_2", "end_2", "start_1", "end_1"],
        ))

    def test_capture_queue_timeout_raises_capture_error(self):
        with CaptureQueue.acquire(timeout=5.0):
            with self.assertRaises(CaptureError):
                with CaptureQueue.acquire(timeout=0.1):
                    pass


@unittest.skipUnless(importlib.util.find_spec("playwright"), "Playwright is not installed")
class CookieNoticeBrowserTests(unittest.TestCase):
    def test_offer_wait_handles_notice_arriving_after_initial_check(self):
        from playwright.sync_api import sync_playwright

        with sync_playwright() as playwright:
            try:
                browser = playwright.chromium.launch(headless=True)
            except Exception as exc:
                self.skipTest(f"Chromium unavailable: {type(exc).__name__}")
            try:
                page = browser.new_page()
                page.set_content("""<div id='corePrice_feature_div'>EGP 100</div>
                    <div id='desktop_buybox'><div class='offer-display-features-container'
                    style='display:none'>Seller</div></div>""")
                page.evaluate("""() => setTimeout(() => {
                    const notice = document.createElement('div');
                    notice.id = 'sp-cc';
                    notice.style.cssText = 'position:fixed;inset:0;background:white;z-index:999';
                    notice.innerHTML = '<h2>Select your cookie preferences</h2><button>Decline</button>';
                    notice.querySelector('button').addEventListener('click', () => {
                        document.querySelector('.offer-display-features-container').style.display = 'block';
                        notice.remove();
                    });
                    document.body.appendChild(notice);
                }, 150)""")
                _, price, seller = wait_for_offer_regions(page, timeout_ms=2000)
                self.assertIsNotNone(price)
                self.assertIsNotNone(seller)
                self.assertFalse(cookie_notice_visible(page))
            finally:
                browser.close()

    def test_delayed_notice_is_declined_and_seller_uncovered(self):
        from playwright.sync_api import sync_playwright

        with sync_playwright() as playwright:
            try:
                browser = playwright.chromium.launch(headless=True)
            except Exception as exc:
                self.skipTest(f"Chromium unavailable: {type(exc).__name__}")
            try:
                page = browser.new_page(viewport={"width": 800, "height": 600})
                page.set_content("<div id='seller' style='position:absolute;top:300px;left:600px'>Seller</div>")
                page.evaluate("""() => setTimeout(() => {
                    const notice = document.createElement('div');
                    notice.id = 'sp-cc';
                    notice.style.cssText = 'position:fixed;inset:0;background:white;z-index:999';
                    notice.innerHTML = '<h2>Select your cookie preferences</h2>' +
                        '<button onclick="this.parentElement.remove()">Decline</button>';
                    document.body.appendChild(notice);
                }, 150)""")
                self.assertTrue(dismiss_cookie_notice(page, wait_ms=1500))
                self.assertFalse(cookie_notice_visible(page))
                self.assertTrue(element_unobstructed(page.locator("#seller")))

                page.set_content("""<div id='seller' style='position:absolute;top:300px;left:600px'>Seller</div>
                    <div id='sp-cc' style='position:fixed;inset:0;background:white;z-index:999'>
                    <h2>Select your cookie preferences</h2><button>Accept</button></div>""")
                self.assertFalse(element_unobstructed(page.locator("#seller")))
                with self.assertRaisesRegex(CaptureError, "Decline button"):
                    dismiss_cookie_notice(page, wait_ms=0)
            finally:
                browser.close()


if __name__ == "__main__":
    unittest.main()
