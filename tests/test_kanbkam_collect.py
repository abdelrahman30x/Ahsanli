import unittest
from decimal import Decimal
from pathlib import Path
from tempfile import TemporaryDirectory

from kanbkam_collect import (Card, Category, Detail, clean_merchant_url, open_database, parse_filter_page,
                             parse_home_categories, parse_product_detail,
                             save_categories, save_detail, save_page)


class KanbkamParserTests(unittest.TestCase):
    def test_home_category_tree(self):
        links = "".join(
            f'<li><a href="https://www.kanbkam.com/eg/ar/category-{i}/l">Leaf {i}</a></li>'
            for i in range(10)
        )
        html = ('<nav class="cd-dropdown"><ul class="cd-dropdown-content">'
                '<li class="has-children"><a href="https://www.kanbkam.com/eg/ar/phones/p">Phones</a>'
                f'<ul>{links}</ul></li></ul></nav>')
        rows = parse_home_categories(html)
        self.assertEqual(len([row for row in rows if row.is_leaf]), 10)
        self.assertEqual(next(row.parent_slug for row in rows if row.slug == "category-0"), "phones")

    def test_filter_cards_and_empty_end_page(self):
        html = "".join(
            f'<li class="thumbnail-new"><a data-gtmid="product-details|{market}|eg|ar" '
            f'href="https://www.kanbkam.com/eg/ar/item-{product_id}">'
            f'<div class="caption"><h3>{market} phone</h3><p>1,299.50 جنية مصرى</p></div></a></li>'
            for market, product_id in (("amazon", "B0CGT6RLT7"), ("noon", "N70278017V"))
        )
        cards, count = parse_filter_page({"success": True, "html": html, "count": 2, "skip": "0"})
        self.assertEqual(count, 2)
        self.assertEqual([row.marketplace for row in cards], ["amazon", "noon"])
        self.assertEqual(cards[0].price, Decimal("1299.50"))
        self.assertEqual(parse_filter_page({"success": True, "html": "", "count": 0, "skip": "100000000"}), ([], 0))
        with self.assertRaises(ValueError):
            parse_filter_page({"success": True, "html": html, "count": 3})

    def test_product_brand_and_hidden_stock_marker(self):
        html = ('<link rel="canonical" href="https://www.kanbkam.com/eg/ar/pixel-B0CGT6RLT7">'
                '<h1>Pixel 8</h1><script type="application/ld+json">'
                '[{"@type":"Product","brand":{"name":"Google"}}]</script>'
                '<article class="merchants"><span id="priceNumber">36,988</span>'
                '<div class="buy-now"><a href="https://www.amazon.eg/dp/B0CGT6RLT7">Buy</a></div>'
                '<span class="out-stock" style="display:none">المنتج غير متوفر</span></article>')
        detail = parse_product_detail(html, "https://www.kanbkam.com/eg/ar/pixel-B0CGT6RLT7")
        self.assertEqual(detail.brand, "Google")
        self.assertEqual(detail.price, Decimal("36988.00"))
        self.assertEqual(detail.stock_status, "in_stock")
        self.assertEqual(detail.merchant_url, "https://www.amazon.eg/dp/B0CGT6RLT7")
        unavailable = html.replace('style="display:none"', 'style=""')
        self.assertEqual(parse_product_detail(unavailable, "https://www.kanbkam.com/eg/ar/pixel-B0CGT6RLT7").stock_status,
                         "out_of_stock")

    def test_store_urls_are_identity_checked_and_tracking_removed(self):
        amazon = clean_merchant_url("amazon", "B0CGT6RLT7",
            "https://www.amazon.eg/dp/B0CGT6RLT7?tag=other-21&ref=kanbkam")
        noon = clean_merchant_url("noon", "N70278017V",
            "https://www.noon.com/ar-eg/pixel-8/N70278017V/p?ref=kanbkam")
        jumia = clean_merchant_url("jumia", "jmpg25055",
            "https://www.jumia.com.eg/ar/iphone-case-mpg25055.html?ref=kanbkam")
        self.assertEqual(amazon, "https://www.amazon.eg/dp/B0CGT6RLT7")
        self.assertEqual(noon, "https://www.noon.com/egypt-ar/pixel-8/N70278017V/p/")
        self.assertEqual(jumia, "https://www.jumia.com.eg/ar/iphone-case-mpg25055.html")
        self.assertEqual(clean_merchant_url("noon", "N70278017V",
            "https://www.noon.com/ar-eg/pixel-8/N00000000V/p"), "")
        self.assertEqual(clean_merchant_url("amazon", "B0CGT6RLT7",
            "https://evil.example/dp/B0CGT6RLT7"), "")

    def test_detail_saves_direct_merchant_url(self):
        with TemporaryDirectory() as directory:
            con = open_database(Path(directory) / "collector.sqlite3")
            save_categories(con, [Category("mobile-phone", "Phones", "", None, True, 1)])
            card = Card("noon", "N70278017V", "https://www.kanbkam.com/eg/ar/pixel-N70278017V",
                        "Pixel", Decimal("100.00"))
            save_page(con, "mobile-phone", 0, [card], 1)
            save_detail(con, "noon", card.product_id,
                Detail("Pixel", "Google", Decimal("100.00"), "in_stock",
                    "https://www.noon.com/egypt-ar/pixel/N70278017V/p/"))
            self.assertEqual(con.execute("SELECT merchant_url FROM products").fetchone()[0],
                             "https://www.noon.com/egypt-ar/pixel/N70278017V/p/")
            con.close()

    def test_home_refresh_preserves_page_cursor_and_initial_price_baseline(self):
        with TemporaryDirectory() as directory:
            con = open_database(Path(directory) / "collector.sqlite3")
            category = Category("mobile-phone", "Phones", "https://www.kanbkam.com/eg/ar/mobile-phone/l", None, True, 1)
            save_categories(con, [category])
            card = Card("amazon", "B0CGT6RLT7", "https://www.kanbkam.com/eg/ar/pixel-B0CGT6RLT7",
                        "Pixel 8", Decimal("36988.00"))
            save_page(con, "mobile-phone", 0, [card], 1)
            save_categories(con, [Category(category.slug, "Mobile phones", category.url, None, True, 1)])
            row = con.execute("SELECT next_skip,pages_scanned,name FROM categories WHERE slug='mobile-phone'").fetchone()
            self.assertEqual((row["next_skip"], row["pages_scanned"], row["name"]), (1, 1, "Mobile phones"))
            self.assertEqual(con.execute("SELECT COUNT(*) FROM price_changes").fetchone()[0], 0)
            con.close()


if __name__ == "__main__":
    unittest.main()
