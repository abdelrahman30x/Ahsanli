"""Create and validate a manifest for a completed mobile catalog refresh."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path

from verify_mobile_snapshot import verify


FILES = (
    "mobile_catalog.json",
    "mobile_audit.json",
    "mobile_urls.txt",
    "mobile_prices.json",
    "mobile_review.json",
)


def create_manifest(directory: Path) -> dict:
    catalog = json.loads((directory / "mobile_catalog.json").read_text(encoding="utf-8"))
    audit = json.loads((directory / "mobile_audit.json").read_text(encoding="utf-8"))
    state = json.loads((directory / "mobile_prices.json").read_text(encoding="utf-8"))
    urls = (directory / "mobile_urls.txt").read_text(encoding="utf-8").splitlines()
    classes = Counter(row["classification"] for row in audit["records"].values())
    manifest = {
        "snapshot_created_at_utc": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "source_url": catalog["source_url"],
        "catalog_collected_at_utc": catalog["collected_at_utc"],
        "pages_scanned": catalog["pages_scanned"],
        "catalog_results": catalog["unique_products"],
        "phone_urls": len(urls),
        "excluded_results": classes["other"],
        "review_results": classes["unknown"],
        "products_with_price_state": sum(bool(row.get("price")) for row in state["products"].values()),
        "files_sha256": {
            name: hashlib.sha256((directory / name).read_bytes()).hexdigest() for name in FILES
        },
    }
    target = directory / "manifest.json"
    temporary = target.with_name(target.name + ".tmp")
    temporary.write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    os.replace(temporary, target)
    return verify(directory)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    print(json.dumps(create_manifest(args.directory), ensure_ascii=False))
