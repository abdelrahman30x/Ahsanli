import csv
import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, patch
from urllib.error import HTTPError

import google_ads_market_research as research


def creative(owner="AR1", identity="CR1", text=""):
    return {"1": owner, "2": identity, "3": {"3": {"2": text}}, "12": "Agency"}


class ResearchTests(unittest.TestCase):
    def test_pasted_http_response_and_xssi(self):
        self.assertEqual(research.parse_response('HTTP/2 200 OK\r\nContent-Type: application/json\r\n\r\n)]}\'\n{"1":[]}')["1"], [])
        with self.assertRaises(ValueError):
            research.parse_response("<html>Verification</html>")

    def test_full_account_query_clears_domain_filter(self):
        domain = research.Query("domain", "promo3.info").payload()
        account = research.Query("advertiser", "AR1").payload("next")
        self.assertEqual(domain["3"]["12"]["1"], "promo3.info")
        self.assertNotIn("13", domain["3"])
        self.assertEqual(account["3"], {"12": {"1": "", "2": True}, "13": {"1": ["AR1"]}})
        self.assertEqual(account["4"], "next")

    def test_domain_can_discover_multiple_advertisers_and_resume_chain(self):
        query = research.Query("domain", "promo3.info")
        client = Mock()
        client.search.side_effect = [{"1": [creative()], "2": "cursor"}, research.CollectionStopped("HTTP 429")]
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            ads, info = research.collect_query(client, query, folder)
            self.assertFalse(info["complete"])
            self.assertEqual(len(ads), 1)
            resumed = Mock()
            resumed.search.return_value = {"1": [creative("AR2", "CR2")]}
            ads, info = research.collect_query(resumed, query, folder)
            self.assertTrue(info["complete"])
            self.assertEqual({a["1"] for a in ads}, {"AR1", "AR2"})
            resumed.search.assert_called_once_with(query, "cursor")
            offline = Mock()
            same_ads, complete = research.collect_query(offline, query, folder, offline=True)
            self.assertEqual(len(same_ads), 2)
            self.assertTrue(complete["complete"])
            offline.search.assert_not_called()

    def test_page_limit_repeated_cursor_and_foreign_owner_stay_incomplete(self):
        cases = [
            ([{"1": [creative()], "2": "n"}], 1),
            ([{"1": [creative()], "2": "n"}, {"1": [creative(identity="CR2")], "2": "n"}], 0),
            ([{"1": [creative("AR2")]}], 0),
            ([{"1": [], "2": "n"}], 0),
        ]
        for pages, limit in cases:
            with self.subTest(pages=pages), tempfile.TemporaryDirectory() as temporary:
                client = Mock()
                client.search.side_effect = pages
                _, info = research.collect_query(client, research.Query("advertiser", "AR1"), Path(temporary), limit)
                self.assertFalse(info["complete"])
                self.assertIn("error", info)

    def test_composite_deduplication_and_valid_zero_result_domain(self):
        client = Mock()
        client.search.side_effect = [{"1": [creative(), creative("AR2")], "2": "n"},
                                     {"1": [creative(), creative(identity="CR2")]}]
        with tempfile.TemporaryDirectory() as temporary:
            ads, info = research.collect_query(client, research.Query("domain", "promo3.info"), Path(temporary))
            self.assertTrue(info["complete"])
            self.assertEqual(len(ads), 3)
        with tempfile.TemporaryDirectory() as temporary:
            with patch.object(research.RpcClient, "search", return_value={"1": []}):
                self.assertEqual(research.main(["--domain", "empty.example", "--output-dir", temporary]), 0)

    def test_cache_rejects_different_query_or_cursor(self):
        query = research.Query("advertiser", "AR1")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            research.write_json(root / query.key / "page-00001.json", {"request": query.payload("wrong"), "response": {"1": []}})
            client = Mock()
            _, info = research.collect_query(client, query, root)
            self.assertFalse(info["complete"])
            client.search.assert_not_called()

    def test_429_stops_without_cookies_or_retrying(self):
        client = research.RpcClient(delay=0)
        client.opener = Mock()
        client.opener.open.side_effect = HTTPError("https://example.test", 429, "limit", {}, io.BytesIO())
        with self.assertRaises(research.CollectionStopped):
            client.search(research.Query("advertiser", "AR1"), "")
        with self.assertRaises(research.CollectionStopped):
            client.search(research.Query("advertiser", "AR2"), "")
        self.assertEqual(client.opener.open.call_count, 1)
        request = client.opener.open.call_args.args[0]
        self.assertNotIn("cookie", {key.lower() for key in request.headers})

    def test_ocr_reuse_does_not_lose_rich_previews_to_sparse_newer_cache(self):
        item = research.normalize_ad(creative())
        image = "https://tpc.googlesyndication.com/archive/simgad/123"
        with tempfile.TemporaryDirectory() as temporary:
            paths = [Path(temporary) / name for name in ["rich.json", "sparse.json"]]
            research.write_json(paths[0], {"ads": [{**item, "preview_image_url": image, "ocr_text": "Noon code"}]})
            research.write_json(paths[1], {"ads": [item]})
            self.assertEqual(research.reuse_text([item], paths), 1)
            self.assertEqual(item["ocr_text"], "Noon code")
            self.assertEqual(research.reuse_text([item], paths), 0)
            item["ocr_text"] = "Fresh text matching the new OCR line boxes"
            item["ocr_lines"] = [{"text": item["ocr_text"]}]
            self.assertEqual(research.reuse_text([item], paths), 0)
            self.assertEqual(item["ocr_text"], "Fresh text matching the new OCR line boxes")
            changed = {**item, "preview_image_url": image + "4"}
            changed.pop("ocr_text")
            self.assertEqual(research.reuse_text([changed], paths), 0)
            self.assertNotIn("ocr_text", changed)

    def test_store_product_mentions_unknowns_and_alias_overlap(self):
        ads = [research.normalize_ad(creative(identity="CR" + str(i), text=text)) for i, text in enumerate([
            "احذية اديداس خصم ترينديول", "Nice One coupon", "coupon on Newbrand", "كوبون iHerb", "Homebox code", "", "Agency only"])]
        companies = research.classify(ads, [])
        self.assertEqual(ads[0]["promoted_company"], "Trendyol")
        extra = next(m for m in ads[0]["brand_mentions"] if m["company"] == "Adidas")
        self.assertEqual(extra["role"], "additional_mention_needs_review")
        self.assertEqual([m["company"] for m in ads[1]["brand_mentions"]], ["Niceone"])
        self.assertIsNone(ads[2]["promoted_company"])
        self.assertIsNone(ads[5]["promoted_company"])
        self.assertIsNone(ads[6]["promoted_company"])
        self.assertEqual(ads[3]["promoted_company"], "iHerb")
        self.assertEqual(ads[4]["promoted_company"], "Homebox")
        self.assertFalse(next(c for c in companies if c["name"] == "iHerb")["found_in_supplied_catalog"])

    def test_csv_and_report_escape_untrusted_ad_text(self):
        ads = [research.normalize_ad(creative(text='&lt;script&gt;alert(1)&lt;/script&gt; Noon code'))]
        ads[0]["advertiser_name"] = "<img onerror=alert(1)>"
        companies = research.classify(ads, [])
        summary = {"advertisers": 1, "primary_companies": 1, "processed_at_utc": "now", "unresolved_ads": 0, "additional_brand_mentions": 0}
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            research.export(folder, ads, companies, summary, {"AR1": {"complete": True}}, {})
            report = (folder / "report.html").read_text(encoding="utf-8")
            self.assertNotIn("<img onerror=", report)
            self.assertNotIn("<script>alert(1)", report)
            research.write_csv(folder / "safe.csv", [{"name": "=HYPERLINK()"}], ["name"])
            with (folder / "safe.csv").open(encoding="utf-8-sig") as handle:
                row = list(csv.DictReader(handle))[0]
            self.assertEqual(row["name"], "'=HYPERLINK()")

    def test_saved_full_snapshot_entrypoint_writes_every_ad_without_network(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "saved.json"
            research.write_json(source, {"summary": {"complete": True, "pages": 2}, "ads": [research.normalize_ad(creative(text="Noon code"))]})
            with patch.object(research.RpcClient, "post", side_effect=AssertionError("unexpected network")):
                code = research.main(["--from-ads", str(source), "--offline", "--output-dir", str(root / "out")])
            self.assertEqual(code, 0)
            output = json.loads((root / "out" / "ads_all.json").read_text(encoding="utf-8"))
            self.assertEqual(len(output["ads"]), 1)
            self.assertTrue(output["summary"]["complete"])


if __name__ == "__main__":
    unittest.main()
