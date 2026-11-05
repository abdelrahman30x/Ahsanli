import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from affiliate_companies import build_catalog, fetch_pages, main, parse_html_offers, parse_json_offers


def response(product_id, advertiser_id="10", name="Sample Store"):
    return json.dumps([{
        "productId": product_id,
        "advertiserId": advertiser_id,
        "advertiserName": name,
        "name": f"Campaign {product_id}",
        "countryName": "Egypt",
        "markets": [{"name": "Egypt"}],
        "landingPage": {"websiteUrl": "https://example.com"},
    }])


class AffiliateCompanyTests(unittest.TestCase):
    def test_html_extracts_company_and_offer_details(self):
        raw = ('<table id="list_table"><tbody><tr><td><img src="https://img.example/logo.png">'
               '<h5>Shop &amp; Save | Egypt | CPS | Coupons</h5></td></tr></tbody></table>')
        offers = parse_html_offers(raw)
        self.assertEqual(len(offers), 1)
        self.assertEqual(offers[0]["company_name"], "Shop & Save")
        self.assertEqual(offers[0]["region"], "Egypt")
        self.assertEqual(offers[0]["offer_type"], "Coupons")
        self.assertEqual(offers[0]["details"], ["CPS"])

    def test_duplicate_campaigns_merge_under_company(self):
        offers = parse_json_offers(response(101)[:-1] + "," + response(102)[1:])
        catalog = build_catalog(offers + offers)
        self.assertEqual(catalog["summary"], {
            "companies": 1, "unique_offers": 2, "duplicate_offers_removed": 2, "input_offers": 4,
        })
        company = catalog["companies"][0]
        self.assertEqual(company["name"], "Sample Store")
        self.assertEqual(company["advertiser_ids"], ["10"])
        self.assertEqual(company["markets"], ["Egypt"])
        self.assertEqual(len(company["offers"]), 2)

    def test_three_distinct_pages_are_fetched(self):
        with patch("affiliate_companies.urlopen", side_effect=[
                io.BytesIO(response(1).encode()), io.BytesIO(response(2).encode()),
                io.BytesIO(response(3).encode())]) as open_url:
            offers = fetch_pages("https://api.example/campaigns?page={page}", 3)
        self.assertEqual([offer["product_id"] for offer in offers], [1, 2, 3])
        self.assertEqual([call.args[0].full_url for call in open_url.call_args_list], [
            "https://api.example/campaigns?page=1", "https://api.example/campaigns?page=2",
            "https://api.example/campaigns?page=3",
        ])

    def test_repeated_page_is_not_silently_treated_as_complete(self):
        with patch("affiliate_companies.urlopen", side_effect=[
                io.BytesIO(response(1).encode()), io.BytesIO(response(1).encode())]):
            with self.assertRaisesRegex(ValueError, "repeated an earlier page"):
                fetch_pages("https://api.example/campaigns?page={page}", 2)

    def test_saved_inputs_report_missing_and_repeated_pages(self):
        html = ('<table id="list_table"><tr><td><h5>Shop | Egypt | Links</h5></td></tr></table>'
                '<div id="list_table_info">Showing 1 to 1 of 2 entries</div>')
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "page.html").write_text(html, encoding="utf-8")
            (root / "page.json").write_text(response(1), encoding="utf-8")
            output = root / "companies.json"
            self.assertEqual(main(["--input", str(root / "page.html"),
                                   "--input", str(root / "page.json"),
                                   "--input", str(root / "page.json"),
                                   "--output", str(output)]), 0)
            result = json.loads(output.read_text(encoding="utf-8"))
        self.assertEqual(result["summary"]["companies"], 2)
        self.assertEqual(len(result["summary"]["warnings"]), 2)
        self.assertTrue(any("other pages are missing" in warning
                            for warning in result["summary"]["warnings"]))
        self.assertTrue(any("repeats" in warning
                            for warning in result["summary"]["warnings"]))


if __name__ == "__main__":
    unittest.main()
