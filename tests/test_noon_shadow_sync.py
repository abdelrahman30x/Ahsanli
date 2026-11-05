import subprocess
import unittest
from types import SimpleNamespace
from unittest.mock import patch

from noon_http import NoonHttpError
from noon_shadow_sync import fetch_offer_bounded


class BoundedNoonFetchTests(unittest.TestCase):
    def test_native_request_is_killed_after_timeout(self):
        with patch("noon_shadow_sync.subprocess.run", side_effect=subprocess.TimeoutExpired("python", 35)) as run:
            with self.assertRaisesRegex(NoonHttpError, "35 seconds"):
                fetch_offer_bounded("https://www.noon.com/egypt-en/x/N1234567890/p/")
        self.assertEqual(run.call_args.kwargs["timeout"], 35)

    def test_subprocess_offer_and_clear_failure(self):
        url = "https://www.noon.com/egypt-en/x/N1234567890/p/"
        with patch("noon_shadow_sync.subprocess.run", return_value=SimpleNamespace(
                returncode=0, stdout='{"price":"314","seller":"X-Tech"}', stderr="")):
            self.assertEqual(fetch_offer_bounded(url)["price"], "314")
        with patch("noon_shadow_sync.subprocess.run", return_value=SimpleNamespace(
                returncode=1, stdout="", stderr="Noon check failed: Noon product is out of stock")):
            with self.assertRaisesRegex(NoonHttpError, "out of stock"):
                fetch_offer_bounded(url)


if __name__ == "__main__":
    unittest.main()
