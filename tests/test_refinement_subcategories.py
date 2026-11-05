"""Unit tests for nested category & subcategory refinement parsing."""

from __future__ import annotations

import unittest
from unittest.mock import MagicMock, patch

from category_scraper import (
    CategoryPageResult,
    CategoryRefinementParser,
    CategoryScraper,
    ScrapedCategoryProduct,
    SubCategoryRefinement,
    parse_category_page_html,
    parse_category_refinements,
)

USER_SIDEBAR_SNIPPET = """
<ul aria-labelledby="n-title" class="a-unordered-list a-nostyle a-vertical a-spacing-medium">
    <li class="a-spacing-micro"><span class="a-list-item">
        <a class="a-color-base a-link-normal" href="/-/en/s?rh=n%3A18018102031&amp;dc&amp;qid=1792931059&amp;rnid=21832868031&amp;ref=sr_nr_n_0">
            <span class="apb-browse-back-arrow-icon aok-inline-block" dir="auto"></span>
            <span dir="auto">Electronics</span>
        </a>
    </span></li>
    <li class="a-spacing-micro"><span class="a-list-item">
        <h2 class="a-size-base a-color-base apb-browse-refinements-indent-1 a-text-bold" dir="auto">
            Mobile Phones &amp; Communication
        </h2>
    </span></li>
    <li class="a-spacing-micro apb-browse-refinements-indent-2"><span class="a-list-item">
        <a class="a-color-base a-link-normal" href="/-/en/s?rh=n%3A18018102031%2Cn%3A21832868031%2Cn%3A21832886031&amp;dc&amp;qid=1792931059&amp;rnid=21832868031&amp;ref=sr_nr_n_2">
            <span dir="auto">Accessories</span>
        </a>
    </span></li>
    <li class="a-spacing-micro apb-browse-refinements-indent-2"><span class="a-list-item">
        <a class="a-color-base a-link-normal" href="/-/en/s?rh=n%3A18018102031%2Cn%3A21832868031%2Cn%3A21833029031&amp;dc&amp;qid=1792931059&amp;rnid=21832868031&amp;ref=sr_nr_n_3">
            <span dir="auto">Mobile Broadband</span>
        </a>
    </span></li>
    <li class="a-spacing-micro apb-browse-refinements-indent-2"><span class="a-list-item">
        <a class="a-color-base a-link-normal" href="/-/en/s?rh=n%3A18018102031%2Cn%3A21832868031%2Cn%3A21832883031&amp;dc&amp;qid=1792931059&amp;rnid=21832868031&amp;ref=sr_nr_n_4">
            <span dir="auto">Mobile Phones</span>
        </a>
    </span></li>
    <li class="a-spacing-micro apb-browse-refinements-indent-2"><span class="a-list-item">
        <a class="a-color-base a-link-normal" href="/-/en/s?rh=n%3A18018102031%2Cn%3A21832868031%2Cn%3A21832958031&amp;dc&amp;qid=1792931059&amp;rnid=21832868031&amp;ref=sr_nr_n_5">
            <span dir="auto">Smartwatches</span>
        </a>
    </span></li>
</ul>
"""


class TestSubcategoryRefinementParser(unittest.TestCase):
    def test_parse_sidebar_refinements(self):
        subs = parse_category_refinements(USER_SIDEBAR_SNIPPET, base_url="https://www.amazon.eg")

        self.assertEqual(len(subs), 4)

        # 1. Accessories
        self.assertEqual(subs[0].name, "Accessories")
        self.assertEqual(subs[0].parent_name, "Mobile Phones & Communication")
        self.assertEqual(subs[0].node_id, "21832886031")
        self.assertIn("https://www.amazon.eg/-/en/s?", subs[0].url)
        self.assertIn("21832886031", subs[0].url)

        # 2. Mobile Broadband
        self.assertEqual(subs[1].name, "Mobile Broadband")
        self.assertEqual(subs[1].parent_name, "Mobile Phones & Communication")
        self.assertEqual(subs[1].node_id, "21833029031")

        # 3. Mobile Phones
        self.assertEqual(subs[2].name, "Mobile Phones")
        self.assertEqual(subs[2].parent_name, "Mobile Phones & Communication")
        self.assertEqual(subs[2].node_id, "21832883031")

        # 4. Smartwatches
        self.assertEqual(subs[3].name, "Smartwatches")
        self.assertEqual(subs[3].parent_name, "Mobile Phones & Communication")
        self.assertEqual(subs[3].node_id, "21832958031")

    def test_back_arrow_link_is_ignored(self):
        subs = parse_category_refinements(USER_SIDEBAR_SNIPPET, base_url="https://www.amazon.eg")
        names = [s.name for s in subs]
        self.assertNotIn("Electronics", names)

    def test_parse_category_page_html_includes_subcategories(self):
        full_html = f"""
        <html><body>
            <div id="departments">
                {USER_SIDEBAR_SNIPPET}
            </div>
            <div data-asin="B001122334" class="s-result-item s-asin">
                <h2>
                    <a class="a-link-normal" href="/dp/B001122334">
                        <span class="a-size-medium a-color-base a-text-normal">Test Egyptian Phone</span>
                    </a>
                </h2>
                <div class="a-row">
                    <span class="a-price"><span class="a-offscreen">15,499.00 EGP</span></span>
                </div>
            </div>
        </body></html>
        """
        result = parse_category_page_html(full_html, base_url="https://www.amazon.eg", default_currency="EGP")

        self.assertEqual(len(result.products), 1)
        self.assertEqual(result.products[0].asin, "B001122334")
        self.assertEqual(result.products[0].currency, "EGP")
        self.assertEqual(result.products[0].price, "15499.00")

        self.assertEqual(len(result.subcategories), 4)
        self.assertEqual(result.subcategories[0].name, "Accessories")

    def test_scraper_multipage_pagination(self):
        page1_html = """
        <html><body>
            <div data-asin="B00AAAAAA1" class="s-result-item">
                <span class="a-size-medium">Product One</span>
                <span class="a-price-whole">100</span>
            </div>
            <a class="s-pagination-item s-pagination-next" href="/s?k=test&amp;page=2">Next</a>
        </body></html>
        """
        page2_html = """
        <html><body>
            <div data-asin="B00AAAAAA2" class="s-result-item">
                <span class="a-size-medium">Product Two</span>
                <span class="a-price-whole">200</span>
            </div>
        </body></html>
        """

        def mock_fetcher(url: str) -> str:
            if "page=2" in url:
                return page2_html
            return page1_html

        scraper = CategoryScraper(
            base_url="https://www.amazon.eg",
            default_currency="EGP",
            fetcher=mock_fetcher,
            request_delay_range=(0.0, 0.0),
        )

        products = scraper.scrape_category("https://www.amazon.eg/s?k=test", max_pages=5)
        self.assertEqual(len(products), 2)
        asins = [p.asin for p in products]
        self.assertEqual(asins, ["B00AAAAAA1", "B00AAAAAA2"])


if __name__ == "__main__":
    unittest.main()
