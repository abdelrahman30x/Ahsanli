"""Compare a few HTTP clients from one host without storing page content."""

from __future__ import annotations

import argparse
import time

from amazon_scraper import OutOfStockError, ScrapeError, parse_product_html


def fetch_tls(url: str):
    import tls_client

    session = tls_client.Session(client_identifier="chrome_120", random_tls_extension_order=True)
    return session.get(url, timeout_seconds=20, allow_redirects=True, headers={
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
        "Accept-Language": "en-EG,en;q=0.9",
        "Accept": "text/html,application/xhtml+xml",
    })


def fetch_cloudscraper(url: str):
    import cloudscraper

    return cloudscraper.create_scraper().get(url, timeout=20, allow_redirects=True)


def fetch_curl_cffi(url: str):
    from curl_cffi import requests

    return requests.get(url, impersonate="chrome", timeout=20, allow_redirects=True)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("asins", nargs="+", help="Amazon Egypt ASINs")
    parser.add_argument("--pause", type=float, default=20.0)
    parser.add_argument("--clients", nargs="+", choices=("tls_client", "cloudscraper", "curl_cffi"),
                        default=("tls_client", "cloudscraper", "curl_cffi"))
    args = parser.parse_args()
    if not 0 <= args.pause <= 120:
        parser.error("--pause must be between 0 and 120 seconds")

    available = {"tls_client": fetch_tls, "cloudscraper": fetch_cloudscraper,
                 "curl_cffi": fetch_curl_cffi}
    first = True
    for asin in args.asins:
        if len(asin) != 10 or not asin.isalnum():
            parser.error("Every ASIN must have ten letters or digits")
        for name in args.clients:
            fetch = available[name]
            if not first:
                time.sleep(args.pause)
            first = False
            start = time.monotonic()
            try:
                response = fetch(f"https://www.amazon.eg/dp/{asin}")
                try:
                    result = parse_product_html(response.text, "EGP")
                    outcome = "valid_price_and_seller" if result["seller"] else "missing_seller"
                except OutOfStockError:
                    outcome = "out_of_stock"
                except ScrapeError as exc:
                    outcome = "blocked_page" if "block or CAPTCHA" in str(exc) else "parser_failure"
                print(f"{asin} {name} status={response.status_code} outcome={outcome} "
                      f"seconds={time.monotonic()-start:.1f}", flush=True)
            except Exception as exc:
                print(f"{asin} {name} error={type(exc).__name__} "
                      f"seconds={time.monotonic()-start:.1f}", flush=True)


if __name__ == "__main__":
    main()
