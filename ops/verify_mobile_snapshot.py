"""Validate a mobile data snapshot without modifying it or contacting Amazon."""

from __future__ import annotations

import argparse
import hashlib
import json
from collections import Counter
from pathlib import Path


def verify(directory: Path) -> dict:
    manifest = json.loads((directory / "manifest.json").read_text(encoding="utf-8"))
    data = {}
    for name, expected_hash in manifest["files_sha256"].items():
        raw = (directory / name).read_bytes()
        actual_hash = hashlib.sha256(raw).hexdigest()
        if actual_hash != expected_hash:
            raise ValueError(f"SHA-256 mismatch: {name}")
        data[name] = raw

    catalog = json.loads(data["mobile_catalog.json"])
    audit = json.loads(data["mobile_audit.json"])
    state = json.loads(data["mobile_prices.json"])
    review = json.loads(data["mobile_review.json"])
    urls = [line.strip() for line in data["mobile_urls.txt"].decode("utf-8").splitlines()
            if line.strip()]
    asins = [url.rsplit("/", 1)[-1] for url in urls]
    classes = Counter(record["classification"] for record in audit["records"].values())

    if manifest["source_url"] != catalog["source_url"]:
        raise ValueError("Catalog source does not match manifest")
    if (manifest["pages_scanned"] != catalog["pages_scanned"]
            or catalog["pages_scanned"] != len(catalog["page_listing_counts"])):
        raise ValueError("Catalog page count mismatch")
    if catalog["unique_products"] != len(catalog["products"]) or len(catalog["products"]) != len(audit["records"]):
        raise ValueError("Catalog/audit product count mismatch")
    if len(asins) != len(set(asins)) or len(asins) != classes["phone"]:
        raise ValueError("Phone URL count mismatch or duplicate")
    if any(audit["records"][asin]["classification"] != "phone" for asin in asins):
        raise ValueError("A watched URL is not classified as a phone")
    if not set(state["products"]) <= set(asins):
        raise ValueError("Price state includes products outside the phone watchlist")
    if review["count"] != len(review["products"]) or review["count"] != classes["unknown"]:
        raise ValueError("Review count mismatch")
    counts = {
        "pages_scanned": catalog["pages_scanned"],
        "catalog_results": catalog["unique_products"],
        "phone_urls": len(asins),
        "excluded_results": classes["other"],
        "review_results": classes["unknown"],
        "products_with_price_state": sum(bool(row.get("price")) for row in state["products"].values()),
    }
    for name, actual in counts.items():
        if manifest[name] != actual:
            raise ValueError(f"Manifest count mismatch: {name}")
    return counts


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path, nargs="?",
                        default=Path(__file__).resolve().parent.parent / "data" / "mobiles")
    args = parser.parse_args()
    print(json.dumps(verify(args.directory), ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
