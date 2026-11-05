import base64
import gzip
import contextlib
import io
import json
import tempfile
import unittest
from collections import Counter
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import MagicMock, Mock, patch
from urllib.parse import quote

import ad_text_extraction as extraction
import research_loop as research


def args(directory, **overrides):
    values = dict(output_dir=directory, catalog=directory / "missing.json", import_ads=[], cache_dir=[],
                  offline=False, max_jobs=0, max_depth=None, max_pages=0, cooldown=900,
                  rpc_delay=1, serp_delay=5, ocr_workers=1, english_queries=False, expand_new_names=True)
    values.update(overrides)
    return SimpleNamespace(**values)


def creative(owner, identity, text):
    return {"1": owner, "2": identity, "12": "Agency " + owner,
            "3": {"3": {"2": text}}, "6": {"1": "1700000000"}, "7": {"1": "1793360000"}}


class FakeBackend:
    def __init__(self, partial=None):
        self.calls = Counter()
        self.partial = partial

    def search(self, query):
        self.calls[("search", query)] += 1
        return {"query": query, "status": "ok", "captured_at": "snapshot",
                "ads": [{"domain": "two.test" if "ستايلي" in query else "one.test", "headline": query}]}

    def collection(self, kind, value):
        self.calls[(kind, value)] += 1
        if kind == "domain":
            owner = "AR2" if value == "two.test" else "AR1"
            items = [creative(owner, "CR1", "iHerb Coupon Code")]
        elif value == "AR1":
            items = [creative(value, "CR1", "iHerb Coupon Code"), creative(value, "CR2", "Styli Coupon Code")]
        else:
            items = [creative(value, "CR3", "Noon Discount Code"), creative(value, "CR4", "ZenithPrime Coupon Code")]
        complete = self.partial != (kind, value)
        if not complete:
            self.partial = None
        return items, {"complete": complete, "pages": 1, "unique_ads": len(items), "error": "HTTP 429" if not complete else ""}

    def ocr(self, ads, owner):
        self.calls[("ocr", owner)] += 1
        return ads

    def detail(self, ad):
        raise AssertionError("Unexpected detail request")


class LoopTests(unittest.TestCase):
    def setUp(self):
        redirect = contextlib.redirect_stdout(io.StringIO())
        redirect.__enter__()
        self.addCleanup(redirect.__exit__, None, None, None)
    def test_full_cyclic_graph_exhausts_and_never_refetches_known_account(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            state = research.State(folder / "state.db")
            try:
                backend = FakeBackend()
                loop = research.Loop(state, backend, args(folder))
                loop.store("اي هيرب", 0)
                self.assertEqual(loop.run(), 0)
                summary = json.loads((folder / "status.json").read_text(encoding="utf-8"))
                self.assertTrue(summary["frontier_exhausted"])
                self.assertEqual(summary["advertisers"], 2)
                self.assertEqual(summary["unique_ads"], 4)
                self.assertEqual(backend.calls[("advertiser", "AR1")], 1)
                self.assertEqual(backend.calls[("domain", "one.test")], 1)
                self.assertEqual(state.db.execute("SELECT COUNT(*) FROM jobs WHERE kind='store' AND key='iherb'").fetchone()[0], 1)
                self.assertIsNotNone(state.db.execute("SELECT 1 FROM jobs WHERE kind='store' AND key='zenithprime'").fetchone())
                edges = json.loads((folder / "relationships.json").read_text(encoding="utf-8"))["edges"]
                self.assertTrue(any(edge["source_kind"] == "ad" and edge["target_kind"] == "store" for edge in edges))
                before = dict(backend.calls)
                self.assertEqual(loop.run(), 0)
                self.assertEqual(dict(backend.calls), before)
            finally:
                state.close()

    def test_rate_limited_domain_preserves_discovered_account_and_resumes(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            state = research.State(folder / "state.db")
            try:
                backend = FakeBackend(partial=("domain", "one.test"))
                loop = research.Loop(state, backend, args(folder))
                loop.store("iHerb", 0)
                self.assertEqual(loop.run(), 2)
                self.assertEqual(state.db.execute("SELECT state FROM jobs WHERE kind='domain'").fetchone()[0], "waiting")
                self.assertIsNotNone(state.db.execute("SELECT 1 FROM jobs WHERE kind='advertiser' AND key='AR1'").fetchone())
                self.assertGreater(state.get("paused_until"), 0)
                state.set("paused_until", 0)
                state.db.commit()
                self.assertEqual(loop.run(), 0)
                self.assertEqual(backend.calls[("advertiser", "AR1")], 1)
            finally:
                state.close()

    def test_partial_account_retains_ads_before_pause(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            state = research.State(folder / "state.db")
            try:
                loop = research.Loop(state, FakeBackend(partial=("advertiser", "AR1")), args(folder))
                state.enqueue("advertiser", "AR1", {"advertiser_id": "AR1"}, 0)
                self.assertEqual(loop.run(), 2)
                self.assertEqual(len(state.ads()), 2)
                self.assertFalse(json.loads((folder / "status.json").read_text())["complete"])
            finally:
                state.close()

    def test_job_and_depth_limits_keep_frontier_resumable(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            state = research.State(folder / "state.db")
            try:
                options = args(folder, max_jobs=2)
                loop = research.Loop(state, FakeBackend(), options)
                loop.store("iHerb", 0)
                self.assertEqual(loop.run(), 2)
                self.assertEqual(json.loads((folder / "status.json").read_text())["status"], "job_limit")
                options.max_jobs = 0
                options.max_depth = 0
                self.assertEqual(loop.run(), 2)
                self.assertEqual(json.loads((folder / "status.json").read_text())["status"], "depth_limit")
                options.max_depth = None
                self.assertEqual(loop.run(), 0)
            finally:
                state.close()

    def test_alias_and_contained_brand_names_do_not_expand_into_duplicate_stores(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            state = research.State(folder / "state.db")
            try:
                loop = research.Loop(state, FakeBackend(), args(folder))
                for name in ["iHerb", "اي هيرب", "آي هيرب", "iherb"]:
                    loop.store(name, 1)
                self.assertEqual(state.db.execute("SELECT COUNT(*) FROM jobs").fetchone()[0], 1)
                self.assertEqual(loop.company_name("تطبيق ويجو"), "Wego")
                self.assertEqual(loop.company_name("Farfetch UAE"), "Farfetch")
                self.assertEqual(loop.company_name("دكني"), "DKNY")
            finally:
                state.close()

    def test_search_cookie_adapter_keeps_only_nid_and_uses_no_browser(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            config = folder / "preview.json"
            config.write_text('{"preview_url":"https://www.google.com/search?q=test&adtest=on"}')
            state = research.State(folder / "state.db")
            try:
                options = args(folder, preview_config=config, cookies=folder / "private.json", transport="requests", serp_dir=folder)
                backend = research.Backend(options, state)
                module = MagicMock()
                module.STOP = {"blocked"}
                module.preview_url.return_value = "https://www.google.com/search?q=iherb&adtest=on"
                module.load_cookies.return_value = ([SimpleNamespace(name="NID"), SimpleNamespace(name="SID")], {})
                module.get_direct.return_value = {"query": "iherb", "status": "ok", "ads": []}
                module.cookie_header.side_effect = lambda cookies: "NID=fake" if [c.name for c in cookies] == ["NID"] else self.fail("Unexpected cookie")
                backend.serp = module
                backend.search("iherb")
                module.DirectSession.assert_called_once_with("requests", "chrome", "ar", "NID=fake")
            finally:
                state.close()

    def test_import_is_idempotent_and_never_turns_single_page_into_full_account(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            path = folder / "ads.json"
            ad = research.market.normalize_ad(creative("AR1", "CR1", "Noon Coupon Code"))
            research.market.write_json(path, {"summary": {}, "ads": [ad]})
            state = research.State(folder / "state.db")
            try:
                state.import_ads(path)
                state.import_ads(path)
                self.assertEqual(len(state.ads()), 1)
                self.assertEqual(state.db.execute("SELECT state FROM jobs WHERE kind='advertiser'").fetchone()[0], "pending")
                state.import_capture({"query": "x", "status": "blocked", "ads": []})
                self.assertEqual(state.db.execute("SELECT COUNT(*) FROM captures").fetchone()[0], 0)
            finally:
                state.close()

    def test_output_directory_lock_rejects_second_writer(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            with research.exclusive_run(folder):
                with self.assertRaises(ValueError):
                    with research.exclusive_run(folder):
                        self.fail("Second writer acquired the directory")

    def test_stop_keeps_pending_work_and_does_not_start_cooldown(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            state = research.State(folder / "state.db")
            try:
                loop = research.Loop(state, FakeBackend(), args(folder))
                loop.store("iHerb", 0)
                (folder / "STOP").write_text("stop")
                self.assertEqual(loop.run(), 2)
                self.assertEqual(json.loads((folder / "status.json").read_text())["status"], "stopped")
                self.assertFalse(state.get("paused_until", 0))
                (folder / "STOP").unlink()
                self.assertEqual(loop.run(), 0)
            finally:
                state.close()

    def test_changed_image_invalidates_ocr_and_imported_path_is_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            state = research.State(folder / "state.db")
            try:
                ad = research.market.normalize_ad(creative("AR1", "CR1", ""))
                state.put_ads([{**ad, "preview_image_url": "https://tpc.googlesyndication.com/1", "ocr_text": "old", "promoted_company": "Noon"}])
                state.put_ads([{**ad, "preview_image_url": "https://tpc.googlesyndication.com/2"}])
                self.assertNotIn("ocr_text", state.ads()[0])
                self.assertFalse(state.ads()[0].get("promoted_company"))
                state.put_ads([{**ad, "promoted_company": "Noon"}])
                state.put_ads([{**ad, "promoted_company": None}])
                self.assertIsNone(state.ads()[0]["promoted_company"])
                with self.assertRaises(ValueError):
                    state.put_ads([{**ad, "creative_id": "../../outside"}])
            finally:
                state.close()

    def test_offline_image_work_is_not_marked_complete_and_never_runs_node(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            options = args(folder, offline=True)
            state = research.State(folder / "state.db")
            try:
                ad = research.market.normalize_ad(creative("AR9", "CR99", ""))
                ad["preview_image_url"] = "https://tpc.googlesyndication.com/archive/simgad/100"
                state.put_ads([ad])
                state.enqueue("analyze", "AR9", {"advertiser_id": "AR9"}, 0)
                backend = research.Backend(options, state)
                with patch.object(research.subprocess, "run", side_effect=AssertionError("No Node in offline mode")):
                    self.assertEqual(research.Loop(state, backend, options).run(), 2)
                summary = json.loads((folder / "status.json").read_text())
                self.assertFalse(summary["frontier_exhausted"])
                self.assertEqual(summary["ocr_pending"], 1)
            finally:
                state.close()


class ExtractionTests(unittest.TestCase):
    def test_text_candidates_preserve_all_text_and_label_inference(self):
        ad = {"ocr_text": "promo3.info\nNoon Coupon Code 2026\nSave money with this coupon on your next order.\nUNQ55 30%"}
        fields = extraction.extract_fields(ad)
        self.assertEqual(fields["headline_candidates"], ["Noon Coupon Code 2026"])
        self.assertIn("Save money", fields["description_candidates"][0])
        self.assertTrue(fields["fields_are_inferred"])
        self.assertEqual(fields["all_text"], ad["ocr_text"])
        self.assertIn("UNQ55", fields["coupon_code_candidates"])
        self.assertIn("30%", fields["discount_claims"])
        self.assertEqual(fields["observed_domains"], ["promo3.info"])

    def test_layout_uses_line_height_and_gaps_to_join_wrapped_headline(self):
        lines = [{"text": "Noon coupon", "bbox": {"y0": 20, "y1": 40}},
                 {"text": "code 2026", "bbox": {"y0": 43, "y1": 63}},
                 {"text": "Save money with this coupon on your next order.", "bbox": {"y0": 75, "y1": 87}}]
        fields = extraction.extract_fields({"ocr_text": "\n".join(row["text"] for row in lines), "ocr_lines": lines})
        self.assertEqual(fields["headline_candidates"], ["Noon coupon code 2026"])
        self.assertEqual(fields["field_extraction_method"], "ocr_layout_heuristic")

    def test_publisher_logo_is_not_a_headline_and_sitelink_brands_are_not_primary(self):
        ad = research.market.normalize_ad(creative("AR1", "CR1", ""))
        ad["ocr_text"] = "Coupon Publisher\nwww.couponzil.com\nDiscount coupons for all stores\nFind offers for your next order here.\nNoon coupon Nice One coupon"
        research.market.classify([ad], [])
        fields = extraction.extract_fields(ad)
        extraction.refine_company_relations(ad, fields, [])
        self.assertEqual(fields["headline_candidates"], ["Discount coupons for all stores"])
        self.assertIsNone(ad["promoted_company"])
        self.assertTrue(all(item["role"] == "additional_mention_needs_review" for item in ad["brand_mentions"]))

    def test_unseen_company_name_can_expand_but_generic_phrases_cannot(self):
        good = extraction.new_company_candidates({"headline_candidates": ["ZenithPrime Coupon Code 2026"]})
        self.assertEqual(good[0]["name"], "ZenithPrime")
        for bad in ["Get Coupon Code", "Hotels Worldwide When You Coupon Code", "كوبون خصم 50%", "Online Coupon Code"]:
            self.assertEqual(extraction.new_company_candidates({"headline_candidates": [bad]}), [])

    def test_structured_fields_are_kept_separate_from_ocr_guesses(self):
        fields = extraction.extract_fields({"structured_fields": {"headline": "Real headline", "description": "Real description"}, "ocr_text": "Bad OCR"})
        self.assertEqual(fields["headline_candidates"], ["Real headline"])
        self.assertFalse(fields["fields_are_inferred"])
        self.assertEqual(extraction.overlay_fields("https://untrusted.test/?overlay=abc"), {})


if __name__ == "__main__":
    unittest.main()
