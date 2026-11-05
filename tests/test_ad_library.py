import base64
import hashlib
import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, patch
from urllib.parse import parse_qs

from ad_assets import AssetStore, image_type, valid_url
from affiliate_evidence import Evidence, Page
from company_library import export_library
import google_ads_market_research as market

PNG = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=")
IMAGE = "https://tpc.googlesyndication.com/archive/simgad/123"


class AssetAndLibraryTests(unittest.TestCase):
    def test_original_image_is_saved_hashed_and_reused_for_multiple_accounts(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            ads = [{"preview_image_url": IMAGE}, {"preview_image_url": IMAGE}]
            opener = Mock()
            opener.open.return_value = io.BytesIO(PNG)
            with patch("ad_assets.build_opener", return_value=opener):
                result = AssetStore(folder).save(ads)
            self.assertEqual(result["saved_unique_images"], 1)
            self.assertEqual(opener.open.call_count, 1)
            self.assertEqual((folder / ads[0]["image_relpath"]).read_bytes(), PNG)
            self.assertEqual(ads[0]["image_sha256"], hashlib.sha256(PNG).hexdigest())
            with patch("ad_assets.build_opener", side_effect=AssertionError("No redownload")):
                self.assertEqual(AssetStore(folder).save(ads)["saved_unique_images"], 1)

    def test_cookie_and_tracking_hosts_are_not_used_as_image_sources(self):
        self.assertTrue(valid_url(IMAGE))
        for url in ["http://tpc.googlesyndication.com/a", "https://example.test/image.png", "https://www.google.com/aclk", "https://user:pass@tpc.googlesyndication.com/a"]:
            self.assertFalse(valid_url(url))
        with self.assertRaises(ValueError):
            image_type(b"<html>verification</html>")

    def test_offline_missing_image_stays_explicitly_unsaved(self):
        with tempfile.TemporaryDirectory() as temporary:
            ads = [{"preview_image_url": IMAGE}]
            with patch("ad_assets.build_opener", side_effect=AssertionError("No HTTP offline")):
                result = AssetStore(Path(temporary)).save(ads, offline=True)
            self.assertEqual(result["failed_unique_images"], 1)
            self.assertIn("image_save_error", ads[0])
            self.assertNotIn("image_relpath", ads[0])

    def test_company_first_output_has_local_images_titles_sources_and_multiple_advertisers(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            curated = folder / "sources.json"
            market.write_json(curated, {"programs": [{"company": "Noon", "status": "official_program_published", "source_url": "https://example.test/program", "program_title": "Example official program"}]})
            ads = []
            for owner, identity in [("AR1", "CR1"), ("AR2", "CR2")]:
                ad = market.normalize_ad({"1": owner, "2": identity, "12": "Agency " + owner})
                ad.update(preview_image_url=IMAGE, promoted_company="Noon", brand_mentions=[], ad_text="Noon coupon TEST55",
                          headline_candidates=["Noon coupon TEST55"], description_candidates=["Example description"], extracted_fields={})
                ads.append(ad)
            opener = Mock(); opener.open.return_value = io.BytesIO(PNG)
            with patch("ad_assets.build_opener", return_value=opener):
                AssetStore(folder).save(ads)
            summary = export_library(folder, ads, folder / "missing.json", curated)
            self.assertEqual(summary["companies_with_published_program_source"], 1)
            library = json.loads((folder / "company_library.json").read_text(encoding="utf-8"))
            company = library["companies"][0]
            self.assertEqual(company["advertiser_count"], 2)
            directory = folder / company["directory"]
            self.assertTrue((directory / "index.html").exists())
            saved = json.loads((directory / "ads.json").read_text(encoding="utf-8"))["ads"]
            self.assertEqual((directory / saved[0]["company_image_path"]).read_bytes(), PNG)
            self.assertIn("Example description", (directory / "index.html").read_text(encoding="utf-8"))
            self.assertIsNone(company["current_application_approved"])

    def test_ad_mentions_and_catalog_notapplied_do_not_prove_a_current_program(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            catalog = folder / "catalog.json"
            market.write_json(catalog, {"companies": [{"name": "ExampleBrand", "offers": [{"status": "notapplied"}]}]})
            evidence = Evidence(catalog, folder, folder / "no-curated.json")
            record = evidence.company("ExampleBrand")
            self.assertEqual(record["status"], "catalog_listed_unverified_current")
            self.assertIsNone(record["current_application_approved"])
            self.assertIsNone(record["program_evidence"])
            self.assertEqual(evidence.company("OtherBrand")["status"], "unverified")

    def test_footer_only_program_mentions_cannot_verify_a_merchant(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            directory = Page(); directory.feed('<a href="https://www.arabclicks.com/advertisers/namshi-affiliate-program/">Namshi Affiliate Program</a>')
            unrelated = Page(); unrelated.feed('<h1>Contact us</h1><footer>Namshi Affiliate Program</footer>')
            evidence = Evidence(folder / "none.json", folder, folder / "no-curated.json")
            with patch.object(evidence, "fetch", side_effect=[(directory, "hash"), (unrelated, "hash")]):
                evidence.refresh_network(["Namshi"], delay=0)
            self.assertEqual(evidence.company("Namshi")["status"], "unverified")

    def test_public_network_page_title_is_required_and_ppc_restriction_is_preserved(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            directory = Page(); directory.feed('<a href="https://www.arabclicks.com/advertisers/namshi-affiliate-program/">Namshi Affiliate Program</a>')
            program = Page(); program.feed('<h1>Namshi Affiliate Program</h1><p>You may not bid on the merchant terms.</p>')
            evidence = Evidence(folder / "none.json", folder, folder / "no-curated.json")
            with patch.object(evidence, "fetch", side_effect=[(directory, "hash"), (program, "hash")]):
                evidence.refresh_network(["Namshi"], delay=0)
            self.assertEqual(evidence.company("Namshi")["status"], "network_program_published")
            self.assertIsNone(evidence.company("Namshi")["paid_search_allowed"])
            self.assertFalse(evidence.company("Namshi")["program_evidence"]["brand_bidding_allowed"])

    def test_rpc_curl_request_matches_public_form_without_private_cookies(self):
        response = Mock(status_code=200, content=b'{"1":[]}')
        with patch("curl_cffi.requests.post", return_value=response) as post:
            market.RpcClient(delay=0, transport="curl_cffi").search(market.Query("advertiser", "AR1"), "")
        call = post.call_args
        self.assertTrue(call.args[0].endswith("authuser="))
        self.assertNotIn("cookie", {key.lower() for key in call.kwargs["headers"]})
        self.assertIn("x-same-domain", {key.lower() for key in call.kwargs["headers"]})
        payload = json.loads(parse_qs(call.kwargs["data"].decode())["f.req"][0])
        self.assertEqual(payload["3"]["12"]["1"], "")
        self.assertFalse(call.kwargs["allow_redirects"])

    def test_real_empty_protobuf_response_is_a_valid_zero_result_query(self):
        self.assertEqual(market.parse_response("{}"), {"1": []})
        with self.assertRaises(ValueError):
            market.parse_response('{"error":"denied"}')


if __name__ == "__main__":
    unittest.main()
