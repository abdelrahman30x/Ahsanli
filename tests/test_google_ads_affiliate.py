import base64
import gzip
import io
import json
import tempfile
import unittest
from pathlib import Path
from urllib.parse import quote
from unittest.mock import patch

from google_ads_affiliate import (classify_ads, collect_pages, find_brand, merge_saved_responses,
                                  normalize_ad, overlay_text, use_detail_preview,
                                  request_page)


def ad(creative_id):
    return {
        "1": "AR13632814900081328129",
        "2": creative_id,
        "3": {"3": {"2": '<img src="https://tpc.googlesyndication.com/archive/simgad/123" '
                             'width="364">'}},
        "12": "i Shift L.L.C-FZ",
    }


class GoogleAdsAffiliateTests(unittest.TestCase):
    def test_normalizes_image_preview_without_using_agency_as_brand(self):
        result = normalize_ad(ad("CR123"), "AR13632814900081328129")
        self.assertEqual(result["creative_id"], "CR123")
        self.assertEqual(result["preview_image_url"],
                         "https://tpc.googlesyndication.com/archive/simgad/123")
        self.assertEqual(result["visible_text"], "")
        self.assertEqual(result["advertiser_name"], "i Shift L.L.C-FZ")

    def test_extracts_arabic_and_english_brand_examples(self):
        self.assertEqual(find_brand("promo3.info كود خصم جيني 50%", []),
                         ("Jeeny", "known_alias"))
        self.assertEqual(find_brand("tawfiir.com Enjoy 50% Off On Gosport (P86)",
                                    ["GoSport"]), ("GoSport", "catalog_match"))
        self.assertEqual(find_brand("i Shift L.L.C-FZ", ["GoSport"]), ("", ""))
        self.assertEqual(find_brand("كود خصم نون 70%", []), ("Noon", "known_alias"))
        self.assertEqual(find_brand("كود خصم تويو 2026", []), ("ToYou", "known_alias"))
        self.assertEqual(find_brand("كوبونات خصم H&M 2026", []), ("H&M", "known_alias"))
        self.assertEqual(find_brand("كود خصم فوغا كلوسيت", []), ("VogaCloset", "known_alias"))
        self.assertEqual(find_brand("Descuento Hacoo 2026", []), ("Hacoo", "known_alias"))
        self.assertEqual(find_brand("كود خصم ويجو للطيران", []), ("Wego", "known_alias"))
        self.assertEqual(find_brand("كود ماماز اند باباز 2026", []),
                         ("Mamas and Papas", "known_alias"))
        self.assertEqual(find_brand("اكواد خصم بلومينغديلز", []),
                         ("Bloomingdales", "known_alias"))
        self.assertEqual(find_brand("كود أمريكان أيجل100ر.س", []),
                         ("American Eagle", "known_alias"))
        self.assertEqual(find_brand("خصم ريفال", []), ("ريفال", "known_alias"))

    def test_store_takes_precedence_over_a_product_brand_in_ad_text(self):
        self.assertEqual(find_brand("احذية اديداس الاصلية بخصم ترينديول", []),
                         ("Trendyol", "known_alias"))

    def test_pattern_only_match_requires_review(self):
        item = normalize_ad(ad("CR123"), "AR13632814900081328129")
        item["ocr_text"] = "Enjoy 50% Off On Newbrand (P86)"
        matches, companies, review = classify_ads([item], [])
        self.assertEqual(matches, [])
        self.assertEqual(companies, [])
        self.assertEqual(review[0]["suggested_company"], "Newbrand")
        self.assertIsNone(item["promoted_company"])

    def test_company_list_marks_affiliate_catalog_membership(self):
        item = normalize_ad(ad("CR123"), "AR13632814900081328129")
        item["ocr_text"] = "Noon Discount Code"
        matches, companies, review = classify_ads([item], ["Noon"])
        self.assertEqual(len(matches), 1)
        self.assertEqual(review, [])
        self.assertTrue(companies[0]["found_in_supplied_catalog"])

    def test_saved_response_merges_only_new_creatives(self):
        ads = [normalize_ad(ad("CR123"), "AR13632814900081328129")]
        with tempfile.TemporaryDirectory() as temp:
            response = Path(temp) / "response.json"
            response.write_text(json.dumps({"1": [ad("CR123"), ad("CR456")]}),
                                encoding="utf-8")
            added = merge_saved_responses(ads, [response], "AR13632814900081328129")
        self.assertEqual(added, 1)
        self.assertEqual([item["creative_id"] for item in ads], ["CR123", "CR456"])

    def test_detail_variant_fills_missing_image(self):
        item = normalize_ad({"2": "CR123"}, "AR13632814900081328129")
        detail = {"1": {"5": [{"3": {"2":
            '<img src="https://tpc.googlesyndication.com/archive/simgad/456">'}}]}}
        self.assertTrue(use_detail_preview(item, detail))
        self.assertEqual(item["preview_image_url"],
                         "https://tpc.googlesyndication.com/archive/simgad/456")

    def test_detail_overlay_extracts_structured_ad_text(self):
        def varint(value):
            output = bytearray()
            while value > 127:
                output.append((value & 127) | 128)
                value >>= 7
            output.append(value)
            return bytes(output)

        def field(number, value):
            return varint((number << 3) | 2) + varint(len(value)) + value

        entry = field(1, b"headline") + field(2, field(1, "كود خصم نون".encode()))
        encoded = base64.urlsafe_b64encode(gzip.compress(field(9, entry))).decode()
        url = "https://displayads-formats.googleusercontent.com/ads/preview/content.js?overlay=" + quote("=" + encoded)
        item = normalize_ad({"2": "CR123"}, "AR13632814900081328129")
        self.assertEqual(overlay_text(url), "كود خصم نون")
        self.assertTrue(use_detail_preview(item, {"1": {"5": [{"1": {"4": url}}]}}))
        self.assertEqual(item["visible_text"], "كود خصم نون")

    def test_continuation_token_collects_and_deduplicates_ads(self):
        pages = [
            {"1": [ad("CR123"), ad("CR456")], "2": "next", "4": "800", "5": "900"},
            {"1": [ad("CR456"), ad("CR789")], "4": "800", "5": "900"},
        ]
        with patch("google_ads_affiliate.request_page", side_effect=pages) as request:
            ads, summary = collect_pages("AR13632814900081328129", delay=0)
        self.assertEqual([item["creative_id"] for item in ads], ["CR123", "CR456", "CR789"])
        self.assertEqual(summary["pages"], 2)
        self.assertTrue(summary["complete"])
        self.assertEqual(request.call_args_list[1].args[1], "next")

    def test_repeated_page_does_not_claim_completion(self):
        page = {"1": [ad("CR123")], "2": "next"}
        with patch("google_ads_affiliate.request_page", side_effect=[page, page]):
            with self.assertRaisesRegex(ValueError, "repeated earlier ads"):
                collect_pages("AR13632814900081328129", delay=0)

    def test_request_uses_no_cookies(self):
        response = io.BytesIO(b'{"1":[],"2":""}')
        with patch("google_ads_affiliate.urlopen", return_value=response) as open_url:
            request_page("AR13632814900081328129", "page-two")
        request = open_url.call_args.args[0]
        self.assertNotIn("cookie", {key.lower() for key in request.headers})
        self.assertIn(b"page-two", request.data)


if __name__ == "__main__":
    unittest.main()
