import json
import os
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from noon_http import NoonHttpError, fetch_noon_offer, parse_noon_product_html


URL = ("https://www.noon.com/egypt-en/wireless-over-ear-headphones-black/"
       "N39519778A/p/?o=f308266460c64b9a")
FIXTURE = Path(__file__).parent / "fixtures" / "noon_product_jsonld.html"


class NoonHttpTests(unittest.TestCase):
    def test_follows_only_same_sku_same_host_redirects(self):
        first = URL.replace("/egypt-en/", "/ar-eg/")
        second = URL.replace("/egypt-en/", "/egypt-ar/").split("?")[0].rstrip("/")
        final = second + "/"
        redirects = [
            SimpleNamespace(status_code=302, url=first, headers={"Location": second}, content=b""),
            SimpleNamespace(status_code=307, url=second, headers={"Location": final}, content=b""),
            SimpleNamespace(status_code=200, url=final, headers={}, content=FIXTURE.read_bytes()),
        ]
        class FakeSession:
            def __init__(self, **kwargs):
                self.responses = iter(redirects)
            def get(self, url, **kwargs):
                return next(self.responses)
        fake_module = SimpleNamespace(Session=FakeSession,
                                      exceptions=SimpleNamespace(TLSClientException=RuntimeError))
        with patch.dict(sys.modules, {"tls_client": fake_module}):
            self.assertEqual(fetch_noon_offer(first)["price"], "315.00")
        for location in ("https://evil.example/egypt-en/x/N39519778A/p/",
                         "https://www.noon.com/egypt-en/x/N00000000A/p/"):
            class RedirectSession:
                def __init__(self, **kwargs): pass
                def get(self, url, **kwargs):
                    return SimpleNamespace(status_code=302, url=url, headers={"Location": location}, content=b"")
            fake_module.Session = RedirectSession
            with patch.dict(sys.modules, {"tls_client": fake_module}):
                with self.assertRaisesRegex(NoonHttpError, "redirected away"):
                    fetch_noon_offer(URL)

    def test_fetch_uses_tls_http_only_and_rejects_bad_responses(self):
        response = SimpleNamespace(status_code=200, url=URL, content=FIXTURE.read_bytes())
        calls = []

        class FakeSession:
            def __init__(self, **kwargs):
                calls.append(("session", kwargs))

            def get(self, url, **kwargs):
                calls.append(("get", url, kwargs))
                return response

        fake_module = SimpleNamespace(Session=FakeSession,
                                      exceptions=SimpleNamespace(TLSClientException=RuntimeError))
        with patch.dict(sys.modules, {"tls_client": fake_module}):
            self.assertEqual(fetch_noon_offer(URL)["price"], "315.00")
            self.assertEqual(calls, [
                ("session", {"client_identifier": "chrome_152",
                             "random_tls_extension_order": True}),
                ("get", URL, {"timeout_seconds": 15, "allow_redirects": False}),
            ])
            fetch_noon_offer(URL, proxy_url="http://localhost:8080")
            self.assertEqual(calls[-1][2]["proxy"], "http://localhost:8080")
            response.url = URL.replace("N39519778A", "N00000000A")
            with self.assertRaisesRegex(NoonHttpError, "redirected"):
                fetch_noon_offer(URL)
            response.url = URL
            response.status_code = 403
            with self.assertRaisesRegex(NoonHttpError, "403"):
                fetch_noon_offer(URL)
            response.status_code = 200
            response.content = b"<html>Checking your browser</html>"
            with self.assertRaisesRegex(NoonHttpError, "verification page"):
                fetch_noon_offer(URL)
            response.content = b"x" * 5_000_001
            with self.assertRaisesRegex(NoonHttpError, "too large"):
                fetch_noon_offer(URL)
            fake_module.Session = lambda **_kwargs: (_ for _ in ()).throw(RuntimeError("TLS failed"))
            with self.assertRaisesRegex(NoonHttpError, "request failed"):
                fetch_noon_offer(URL)

    def test_reads_same_product_price_currency_and_seller(self):
        result = parse_noon_product_html(FIXTURE.read_text(encoding="utf-8"), URL)
        self.assertEqual(result, {
            "title": "Wireless Over-Ear Headphones Black",
            "price": "315.00", "currency": "EGP", "seller": "X-Tech", "url": URL,
        })

    def test_rejects_verification_page_and_unusable_offers(self):
        html = FIXTURE.read_text(encoding="utf-8")
        with self.assertRaisesRegex(NoonHttpError, "verification page"):
            parse_noon_product_html("<html><body>Checking your browser</body></html>", URL)
        for changed in (
            html.replace("X-Tech", ""),
            html.replace("EGP", "USD"),
            html.replace("InStock", "OutOfStock"),
            html.replace('"availability":"https://schema.org/InStock",', ""),
            html.replace("315.0", "0.0"),
            html.replace("N39519778A/p/", "N00000000A/p/"),
        ):
            with self.subTest(changed=changed[:80]):
                with self.assertRaises(NoonHttpError):
                    parse_noon_product_html(changed, URL)

    def test_out_of_stock_is_not_misclassified_as_source_verification(self):
        html = FIXTURE.read_text(encoding="utf-8")
        with self.assertRaisesRegex(NoonHttpError, "out of stock"):
            parse_noon_product_html(html.replace("InStock", "OutOfStock"), URL)

    def test_rejects_ambiguous_offers(self):
        html = FIXTURE.read_text(encoding="utf-8")
        first = html.index("<script type=\"application/ld+json\">")
        script = html[first:].split("\n", 2)[1]
        second = json.loads(script)
        second["offers"]["price"] = 320
        html = html.replace("</script>",
                            f'</script><script type="application/ld+json">{json.dumps(second)}</script>', 1)
        with self.assertRaisesRegex(NoonHttpError, "unambiguous"):
            parse_noon_product_html(html, URL)

    @unittest.skipUnless(os.environ.get("RUN_LIVE_NOON_HTTP_TESTS") == "1",
                         "set RUN_LIVE_NOON_HTTP_TESTS=1 for a real Noon HTTP read")
    def test_live_http_offer_without_browser_or_telegram(self):
        url_file = Path(__file__).resolve().parents[1] / "noon_urls.txt"
        url = next(line.split("\t")[0] for line in url_file.read_text(encoding="utf-8").splitlines()
                   if line.startswith("https://"))
        offer = fetch_noon_offer(url)
        self.assertGreater(float(offer["price"]), 0)
        self.assertEqual(offer["currency"], "EGP")
        self.assertTrue(offer["seller"])


if __name__ == "__main__":
    unittest.main()
