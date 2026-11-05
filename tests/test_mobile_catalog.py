"""Tests for complete, verified mobile-category pagination."""

import unittest

from mobile_catalog import crawl_mobile_catalog, selected_page_number


def listing(page: int, asin: str, *, next_page: bool) -> str:
    next_link = '<a class="s-pagination-next" href="?page=2">Next</a>' if next_page else ""
    return (f'<div data-asin="{asin}"><h2><span class="a-text-normal">Phone {page}</span></h2>'
            f'<span class="a-offscreen">EGP 100.00</span></div>'
            f'<span class="s-pagination-selected">{page}</span>{next_link}')


class MobileCatalogTests(unittest.TestCase):
    def test_pagination_collects_unique_asins_and_stops_at_last_page(self):
        pages = {
            1: listing(1, "B012345678", next_page=True),
            2: listing(2, "B012345679", next_page=False),
        }
        fetch = lambda url: pages[2] if "page=2" in url else pages[1]
        catalog = crawl_mobile_catalog(fetch, max_pages=3, delay=0)
        self.assertEqual(catalog["pages_scanned"], 2)
        self.assertEqual(catalog["unique_products"], 2)
        self.assertEqual(catalog["page_listing_counts"], [1, 1])

    def test_rejects_wrong_or_repeated_page(self):
        self.assertEqual(selected_page_number(listing(1, "B012345678", next_page=True)), 1)
        with self.assertRaisesRegex(RuntimeError, "Page 2"):
            crawl_mobile_catalog(lambda _: listing(1, "B012345678", next_page=True),
                                 max_pages=3, delay=0)
        with self.assertRaisesRegex(RuntimeError, "Page 1"):
            crawl_mobile_catalog(lambda _: '<html><title>Verify</title></html>',
                                 max_pages=3, delay=0)


if __name__ == "__main__":
    unittest.main()
