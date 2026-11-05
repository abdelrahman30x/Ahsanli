import hashlib
import json
import sqlite3
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import iherb_competitors as app
import iherb_report as report


class IherbResearchTests(unittest.TestCase):
    def ad(self, title="كود خصم اي هيرب", raw="الوصف الأصلي", **extra):
        return {"advertiser_id":"AR123", "creative_id":"CR456", "advertiser_name":"Publisher",
                "creative_url":"https://adstransparency.google.com/advertiser/AR123/creative/CR456?region=anywhere",
                "structured_fields":{"headline":title,"description":raw},**extra}

    def test_all_spelling_forms(self):
        for phrase in ["iHerb coupon","i herb","كود آي هيرب","ايهيرب خصم","اي هرب","أيهيرب"]:
            self.assertTrue(app.brand_match(phrase),phrase)
        for phrase in ["herbal", "herb", "myiherbcode", "نمشي", "iherbal"]:
            self.assertFalse(app.brand_match(phrase),phrase)

    def test_sitelink_mention_is_not_a_primary_iherb_ad(self):
        role,_=app.relevance(self.ad("كوبونات متاجر متنوعة","خصومات الملابس",ocr_text="كود خصم iHerb"))
        self.assertEqual(role,"additional_mention_needs_review")
        self.assertEqual(app.relevance(self.ad())[0],"headline")

    def test_country_filter_is_distinct_from_viewer_country(self):
        q=app.RegionalQuery("accounts",("AR123",),(2682,),"SA")
        payload=q.payload("cursor")
        self.assertEqual(payload["3"]["8"],[2682])
        self.assertEqual(payload["7"]["3"],2818)
        self.assertEqual(payload["4"],"cursor")
        self.assertEqual(payload["3"]["12"]["1"],"")

    def test_country_cache_and_account_membership_are_isolated(self):
        one=app.RegionalQuery("accounts",("AR123",),(2682,),"SA")
        two=app.RegionalQuery("accounts",("AR123",),(2818,),"EG")
        more=app.RegionalQuery("accounts",("AR123","AR124"),(2682,),"SA")
        self.assertEqual(len({one.key,two.key,more.key}),3)
        self.assertIn("region=SA",one.source_url)

    def test_reject_unscoped_or_invalid_queries(self):
        for values in [("accounts",(),(2682,),"SA"),("accounts",("AR1",),(),"SA"),("domain",("https://example.com",),(2682,),"SA")]:
            with self.assertRaises(ValueError):app.RegionalQuery(*values)

    def test_country_coverage_pending_is_not_zero(self):
        countries=[{"code":"SA","name":"السعودية","geo_id":2682},{"code":"SY","name":"سوريا","geo_id":None}]
        rows=report.country_coverage(countries,[],{"AR123"},[])
        self.assertEqual(rows[0]["status"],"pending")
        self.assertEqual(rows[1]["status"],"unsupported_filter")
        q={"key":"x","kind":"accounts","values":["AR123"],"region":"SA","region_ids":[2682],"complete":True,"creative_keys":[]}
        self.assertEqual(report.country_coverage(countries,[q],{"AR123"},[])[0]["status"],"scanned_known_accounts")
        self.assertEqual(report.country_coverage(countries,[q],{"AR123","AR124"},[])[0]["status"],"partial")

    def test_country_evidence_belongs_to_creative(self):
        ad=self.ad(iherb_relevance="headline",detail_country_codes=["AE"])
        q={"key":"x","kind":"accounts","values":["AR123"],"region":"SA","region_ids":[2682],"complete":True,"creative_keys":[["AR123","CR456"]]}
        report.country_coverage([{"code":"SA","name":"السعودية","geo_id":2682}],[q],{"AR123"},[ad])
        self.assertEqual(ad["country_codes"],["AE","SA"])
        self.assertEqual(ad["country_filter_codes"],["SA"])

    def test_collector_persists_creative_ids_for_country_proof(self):
        with tempfile.TemporaryDirectory() as tmp:
            args=SimpleNamespace(config=app.ROOT/"iherb_competitors.json",country=["SA"],output_dir=Path(tmp),rpc_delay=5,max_pages=0,offline=False,include_official=False)
            research=app.Research(args)
            query=app.RegionalQuery("accounts",("AR123",),(2682,),"SA")
            item={"1":"AR123","2":"CR456","12":"Publisher","3":{"3":{"2":"visible"}}}
            with patch.object(app.market,"collect_query",return_value=([item],{"complete":True})),patch.object(research,"export"):
                research.query(query)
            self.assertEqual(research.store.rows("queries")[0]["creative_keys"],[["AR123","CR456"]])
            self.assertEqual(research.store.rows("queries")[0]["scope"],"Explicit country filters; all available dates")
            research.store.db.close()

    def test_search_preview_changes_preserve_canonical_ocr_and_save_other_preview(self):
        with tempfile.TemporaryDirectory() as tmp:
            args=SimpleNamespace(config=app.ROOT/"iherb_competitors.json",country=["SA"],output_dir=Path(tmp),rpc_delay=5,max_pages=0,offline=False,include_official=False)
            research=app.Research(args)
            old=self.ad(preview_image_url="https://tpc.googlesyndication.com/archive/simgad/1",ocr_text="iherb old text")
            research.store.put([old])
            query=app.RegionalQuery("accounts",("AR123",),(2682,),"SA")
            item={"1":"AR123","2":"CR456","12":"Publisher","3":{"3":{"2":'<img src="https://tpc.googlesyndication.com/archive/simgad/2">'}}}
            with patch.object(app.market,"collect_query",return_value=([item],{"complete":True})),patch.object(research,"export"):
                research.query(query)
            saved=research.store.ads()[0]
            self.assertEqual(saved["ocr_text"],"iherb old text")
            self.assertTrue(saved["preview_image_url"].endswith("/1"))
            self.assertTrue(saved["observed_search_previews"][0]["preview_image_url"].endswith("/2"))
            research.store.db.close()

    def test_report_handles_unanalyzed_changed_image_checkpoint(self):
        with tempfile.TemporaryDirectory() as tmp:
            store=app.Store(Path(tmp))
            ad=self.ad(preview_image_url="https://example.com/1",ocr_text="iherb",iherb_relevance="headline")
            store.put([ad])
            # A prior checkpoint's classification can exist without extracted_fields.
            summary=report.export_report(store,[],{"policy_source":app.POLICY_URL},"checkpoint")
            self.assertEqual(summary["primary_iherb_creatives"],1)
            store.db.close()

    def test_explicit_cookie_export_uses_only_nid_and_search_pause_is_separate(self):
        import sys
        sys.path.insert(0,str(app.DEFAULT_SERP))
        import collect_ads, http_clients
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);cookie_path=root/"cookies.json"
            cookie_path.write_text(json.dumps([{"name":"NID","value":"TEST_ONLY","domain":".google.com","session":True},{"name":"SID","value":"DO_NOT_USE","domain":".google.com","session":True}]),encoding="utf-8")
            args=SimpleNamespace(config=app.ROOT/"iherb_competitors.json",country=["SA"],output_dir=root/"out",rpc_delay=5,max_pages=0,offline=False,include_official=False,serp_dir=app.DEFAULT_SERP,cookies=cookie_path,queries_per_country=1,serp_delay=60)
            research=app.Research(args)
            with patch.object(http_clients,"DirectSession") as transport,patch.object(collect_ads,"get_direct",return_value={"query":"iherb","status":"blocked","ads":[]}),patch.object(research,"export"),patch.object(app.time,"sleep"):
                research.serp()
                self.assertEqual(transport.call_args.args[-1],"NID=TEST_ONLY")
            self.assertFalse(research.rpc.blocked)
            self.assertEqual(research.store.meta("serp_paused")["reason"],"blocked")
            self.assertNotIn("TEST_ONLY",json.dumps(research.store.rows("captures")))
            research.store.db.close()

    def test_changed_image_discards_old_ocr(self):
        with tempfile.TemporaryDirectory() as tmp:
            store=app.Store(Path(tmp))
            store.put([self.ad(preview_image_url="https://example.com/one",ocr_text="old",image_sha256="old")])
            store.put([self.ad(preview_image_url="https://example.com/two")])
            self.assertNotIn("ocr_text",store.ads()[0])
            store.db.close()

    def test_all_detail_variants_and_identity(self):
        variants=[{"3":{"2":f'<img src="https://tpc.googlesyndication.com/archive/simgad/{i}">'}} for i in [1,2,1]]
        detail={"1":{"1":"AR123","2":"CR456","5":variants}}
        out=app.detail_variants(self.ad(),detail)
        self.assertEqual(len(out),2)
        self.assertEqual(len({v["variant_key"] for v in out}),2)
        detail["1"]["2"]="CR999"
        with self.assertRaises(ValueError):app.detail_variants(self.ad(),detail)

    def test_archive_read_only_and_excludes_merchant_by_default(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);source=root/"old.sqlite3"
            c=sqlite3.connect(source);c.execute("CREATE TABLE ads(body TEXT)")
            c.executemany("INSERT INTO ads VALUES(?)",[(json.dumps(self.ad()),),(json.dumps(self.ad(advertiser_id="AR999",advertiser_name="iHerb, LLC")),)])
            c.commit();c.close()
            before=hashlib.sha256(source.read_bytes()).hexdigest()
            store=app.Store(root/"new")
            self.assertEqual(app.import_archive(store,source),1)
            self.assertEqual(hashlib.sha256(source.read_bytes()).hexdigest(),before)
            store.db.close()

    def test_imported_search_does_not_leak_private_preview_fields(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);history=root/"input";history.mkdir()
            (history/"history.json").write_text(json.dumps([{"query":"iherb","status":"ok","ads":[],"url":"PRIVATE","location_token":"PRIVATE","cookies":"PRIVATE"}]),encoding="utf-8")
            store=app.Store(root/"out");app.import_serp(store,history)
            self.assertNotIn("PRIVATE",json.dumps(store.rows("captures")))
            store.db.close()

    def test_draft_limits_original_assets_and_no_sale_claim(self):
        draft=report.campaign_draft([],{},app.POLICY_URL)
        self.assertFalse(draft["policy"]["brand_bidding_allowed_under_standard_terms"])
        for group in draft["groups"]:
            self.assertEqual(len(group["headlines"]),15)
            self.assertEqual(len(group["descriptions"]),4)
            self.assertLessEqual(max(map(len,group["headlines"])),30)
            self.assertLessEqual(max(map(len,group["descriptions"])),90)
            self.assertTrue(all(not app.brand_match(t) for t in group["headlines"]+group["descriptions"]))
            self.assertIsNone(group["final_url"])

    def test_csv_formula_and_html_injection_are_escaped(self):
        self.assertEqual(report.safe_cell(" =HYPERLINK(x)"),"' =HYPERLINK(x)")
        with tempfile.TemporaryDirectory() as tmp:
            store=app.Store(Path(tmp));ad=self.ad("iherb <script>alert(1)</script>")
            role,fields=app.relevance(ad);ad.update(iherb_relevance=role,extracted_fields=fields)
            store.put([ad]);report.export_report(store,[],{"policy_source":app.POLICY_URL},"test")
            markup=(Path(tmp)/"report.html").read_text(encoding="utf-8")
            self.assertNotIn("<script>alert(1)</script>",markup)
            self.assertIn("&lt;script&gt;alert(1)&lt;/script&gt;",markup)
            store.db.close()


if __name__=="__main__":unittest.main()
