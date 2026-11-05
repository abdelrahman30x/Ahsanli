"""Tests for NavMenuParser, NavMenuScraper, and auto-discovery integration."""

from __future__ import annotations

import unittest

from category_scraper import (
    NavMenuParser,
    NavMenuResult,
    NavMenuScraper,
    parse_nav_menu_html,
)


# ---------------------------------------------------------------------------
# Fixture HTML — mirrors real amazon.eg nav structure
# ---------------------------------------------------------------------------

HOMEPAGE_HTML = """
<html><body>
<nav class="hmenu-content">

<!-- Top-level category list -->
<section class="category-section" aria-labelledby="Shop by Category">
  <div class="hmenu-item hmenu-title" role="heading">Shop by Category</div>
  <ul>
    <li><a href="" class="hmenu-item" data-menu-id="3" role="button">
        <div>Mobiles, Tablets &amp; Accessories</div></a></li>
    <li><a href="" class="hmenu-item" data-menu-id="5" role="button">
        <div>TVs &amp; Electronics</div></a></li>
    <li><a href="" class="hmenu-item" data-menu-id="9" role="button">
        <div>Health, Beauty &amp; Perfumes</div></a></li>
  </ul>
</section>

<!-- Sub-category section: Mobiles -->
<section class="category-section" aria-labelledby="Mobiles, Tablets &amp; More">
  <div class="hmenu-item hmenu-title" role="heading">Mobiles, Tablets &amp; More</div>
  <ul>
    <li><a href="/-/en/gp/browse.html?node=21832868031&amp;ref_=nav_em_mob_all"
           class="hmenu-item">All Mobile Phones</a></li>
    <li><a href="/-/en/gp/browse.html?node=21832915031&amp;ref_=nav_em_mob_tablets"
           class="hmenu-item">Tablets</a></li>
    <li><a href="/-/en/gp/browse.html?node=21832878031&amp;ref_=nav_em_mob_wearable"
           class="hmenu-item">Wearable Technology</a></li>
    <li><a href="/-/en/gp/browse.html?node=21833002031&amp;ref_=nav_em_mob_cases"
           class="hmenu-item">Cases &amp; Covers</a></li>
    <li><a href="/-/en/gp/browse.html?node=21833004031&amp;ref_=nav_em_mob_power"
           class="hmenu-item">Power Banks &amp; Chargers</a></li>
    <li><a href="/-/en/gp/browse.html?node=21832886031&amp;ref_=nav_em_mob_acc"
           class="hmenu-item">All Mobile Accessories</a></li>
  </ul>
</section>

<!-- Sub-category section: Electronics -->
<section class="category-section" aria-labelledby="TVs &amp; Electronics">
  <div class="hmenu-item hmenu-title" role="heading">TVs &amp; Electronics</div>
  <ul>
    <li><a href="/-/en/gp/browse.html?node=21834200031&amp;ref_=nav_em_tv_all"
           class="hmenu-item">Televisions</a></li>
    <li><a href="/-/en/gp/browse.html?node=21834201031&amp;ref_=nav_em_tv_audio"
           class="hmenu-item">Audio &amp; Home Theatre</a></li>
  </ul>
</section>

<!-- Section with NO browse-node links — should produce 0 sub-categories -->
<section class="category-section" aria-labelledby="Services">
  <div class="hmenu-item hmenu-title" role="heading">Services</div>
  <ul>
    <li><a href="/prime" class="hmenu-item">Amazon Prime</a></li>
    <li><a href="/giftcards" class="hmenu-item">Gift Cards</a></li>
  </ul>
</section>

</nav>
</body></html>
"""


class TestNavMenuParser(unittest.TestCase):
    def setUp(self):
        self.result = parse_nav_menu_html(HOMEPAGE_HTML, base_url="https://www.amazon.eg")

    def test_sections_discovered(self):
        # "Mobiles" and "TVs & Electronics" have browse-node links; "Services" does not
        self.assertIn("Mobiles, Tablets & More", self.result.sections)
        self.assertIn("TVs & Electronics", self.result.sections)

    def test_services_section_excluded_no_browse_nodes(self):
        # /prime and /giftcards are NOT browse-node URLs → section should have 0 items
        self.assertNotIn("Services", self.result.sections)

    def test_mobiles_subcategory_count(self):
        subs = self.result.sections["Mobiles, Tablets & More"]
        self.assertEqual(len(subs), 6)

    def test_mobiles_subcategory_names(self):
        names = [s.name for s in self.result.sections["Mobiles, Tablets & More"]]
        self.assertIn("All Mobile Phones", names)
        self.assertIn("Tablets", names)
        self.assertIn("Wearable Technology", names)
        self.assertIn("Cases & Covers", names)
        self.assertIn("Power Banks & Chargers", names)
        self.assertIn("All Mobile Accessories", names)

    def test_mobiles_subcategory_urls_are_absolute(self):
        urls = [s.url for s in self.result.sections["Mobiles, Tablets & More"]]
        for url in urls:
            self.assertTrue(url.startswith("https://www.amazon.eg"), url)
            self.assertIn("node=", url)

    def test_electronics_subcategory_count(self):
        subs = self.result.sections["TVs & Electronics"]
        self.assertEqual(len(subs), 2)

    def test_parent_section_field_set_correctly(self):
        for sub in self.result.sections["Mobiles, Tablets & More"]:
            self.assertEqual(sub.parent_section, "Mobiles, Tablets & More")

    def test_all_subcategories_flat_list(self):
        all_subs = self.result.all_subcategories()
        # 6 mobiles + 2 electronics
        self.assertEqual(len(all_subs), 8)

    def test_as_category_dict(self):
        cat_dict = self.result.as_category_dict()
        # Keys should be "Section > Sub" format
        self.assertIn("Mobiles, Tablets & More > All Mobile Phones", cat_dict)
        self.assertIn("TVs & Electronics > Televisions", cat_dict)
        # Values should be full URLs with browse node
        url = cat_dict["Mobiles, Tablets & More > All Mobile Phones"]
        self.assertIn("node=21832868031", url)

    def test_filter_sections_by_partial_name(self):
        filtered = NavMenuScraper.filter_sections(
            self.result,
            ["Mobiles"],
            case_sensitive=False,
        )
        # Only sub-categories from the Mobiles section
        self.assertTrue(all("Mobiles" in k for k in filtered))
        self.assertEqual(len(filtered), 6)

    def test_filter_sections_multiple(self):
        filtered = NavMenuScraper.filter_sections(
            self.result,
            ["Mobiles", "TVs"],
            case_sensitive=False,
        )
        self.assertEqual(len(filtered), 8)  # 6 + 2

    def test_captcha_detection(self):
        with self.assertRaises(Exception):
            parse_nav_menu_html(
                "<html><body>robot check</body></html>",
                base_url="https://www.amazon.eg",
            )


class TestNavMenuScraperFetch(unittest.TestCase):
    """Test NavMenuScraper with a mocked fetcher."""

    def test_fetch_menu_with_mock_fetcher(self):
        def mock_fetcher(url: str) -> str:
            return HOMEPAGE_HTML

        scraper = NavMenuScraper(
            base_url="https://www.amazon.eg",
            default_currency="EGP",
            fetcher=mock_fetcher,
        )
        menu = scraper.fetch_menu()
        self.assertIsInstance(menu, NavMenuResult)
        self.assertGreater(len(menu.sections), 0)
        self.assertEqual(
            len(menu.sections.get("Mobiles, Tablets & More", [])), 6
        )

    def test_as_category_dict_from_fetcher(self):
        def mock_fetcher(url: str) -> str:
            return HOMEPAGE_HTML

        scraper = NavMenuScraper(
            base_url="https://www.amazon.eg",
            fetcher=mock_fetcher,
        )
        menu = scraper.fetch_menu()
        cat_dict = menu.as_category_dict()
        self.assertGreater(len(cat_dict), 0)
        for key, val in cat_dict.items():
            self.assertIn(">", key, f"Key missing section separator: {key}")
            self.assertIn("node=", val, f"URL missing browse node: {val}")


if __name__ == "__main__":
    unittest.main()
