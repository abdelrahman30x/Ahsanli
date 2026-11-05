import unittest

from post_format import format_price_post, is_meaningful_drop


class PricePostTests(unittest.TestCase):
    def test_observed_drop_is_not_claimed_as_store_discount(self):
        post = format_price_post("أمازون", {
            "title": "سماعة\n P9", "old_price": "506.00", "price": "460.00",
            "currency": "EGP", "seller": "Shop", "url": "https://example.test/product",
        })
        self.assertIn("سماعة P9", post)
        self.assertIn("سعر العرض: 460.00 جنيه", post)
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 9.1%", post)
        self.assertNotIn("506.00", post)
        self.assertNotIn("46.00 جنيه", post)
        self.assertNotIn("خصم رسمي", post)
        self.assertNotIn("السعر والتوفر قد يتغيران", post)
        self.assertLessEqual(len(post), 1024)

    def test_verified_amazon_discount_replaces_observed_drop_percentage(self):
        post = format_price_post("أمازون مصر", {
            "title": "Bobana mud mask", "old_price": "60.00", "price": "49.39",
            "currency": "EGP", "seller": "Amazon.eg", "url": "https://www.amazon.eg/dp/B0114C3GDA",
            "store_discount_percent": "21%",
        })
        self.assertIn("الخصم المعروض على صفحة أمازون: 21%", post)
        self.assertNotIn("آخر سعر رصدناه بنسبة", post)
        self.assertNotIn("السعر والتوفر قد يتغيران", post)

    def test_verified_low_stock_is_included_only_when_explicit(self):
        alert = {
            "title": "Kitchen canisters", "old_price": "450.00", "price": "395.99",
            "currency": "EGP", "seller": "Modern2000", "url": "https://www.amazon.eg/dp/B07F7YVDQH",
        }
        self.assertNotIn("آخر 3 قطع", format_price_post("أمازون مصر", alert))
        self.assertIn("آخر 3 قطع", format_price_post("أمازون مصر", {**alert, "stock_count": 3}))
        self.assertIn("آخر قطعتين", format_price_post("أمازون مصر", {**alert, "stock_count": 2}))
        self.assertIn("آخر قطعة", format_price_post("أمازون مصر", {**alert, "stock_count": 1}))
        self.assertNotIn("آخر 12 قطع", format_price_post("أمازون مصر", {**alert, "stock_count": 12}))

    def test_missing_api_seller_is_explicit(self):
        post = format_price_post("أمازون", {
            "title": "Phone", "old_price": "100", "price": "90",
            "currency": "USD", "seller": None, "url": "https://example.test/product",
        })
        self.assertIn("البائع: غير محدد", post)
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 10%", post)

    def test_small_drop_does_not_round_to_zero_percent(self):
        post = format_price_post("نون", {
            "title": "Phone", "old_price": "1000", "price": "999.50",
            "currency": "EGP", "seller": "Shop", "url": "https://example.test/product",
        })
        self.assertIn("أقل من آخر سعر رصدناه بنسبة 0.05%", post)

    def test_meaningful_drop_uses_exact_percentage_boundary(self):
        self.assertFalse(is_meaningful_drop("3500", "3485"))
        self.assertFalse(is_meaningful_drop("100", "90.01"))
        self.assertTrue(is_meaningful_drop("100", "90"))
        self.assertTrue(is_meaningful_drop("3500", "3150"))

    def test_restock_post_formatting(self):
        # Restock without drop (higher or same price)
        post = format_price_post("أمازون مصر", {
            "title": "HONOR X9c 5G", "old_price": "18000.00", "price": "25000.00",
            "currency": "EGP", "seller": "Amazon.eg", "url": "https://www.amazon.eg/dp/B0DMDJ7PZY",
            "is_restock": True,
        })
        self.assertIn("⚡ رجع اتوفر من جديد على أمازون مصر!", post)
        self.assertIn("سعر العرض: 25,000.00 جنيه", post)
        self.assertIn("📦 المنتج أصبح متاحاً للطلب مرة أخرى بعد نفاد الكمية", post)

        # Restock with price drop
        post_drop = format_price_post("أمازون مصر", {
            "title": "HONOR X9c 5G", "old_price": "28000.00", "price": "22000.00",
            "currency": "EGP", "seller": "Amazon.eg", "url": "https://www.amazon.eg/dp/B0DMDJ7PZY",
            "is_restock": True,
        })
        self.assertIn("🔥 رجع اتوفر بسعر مخفّض على أمازون مصر!", post_drop)


if __name__ == "__main__":
    unittest.main()
