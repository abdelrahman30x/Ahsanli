"""Take a consistent online backup of the live legacy tracker database."""

from __future__ import annotations

import argparse
import os
import sqlite3
from datetime import datetime, timezone
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, default=Path("tracker.db"))
    parser.add_argument("--output-dir", type=Path, default=Path("data/legacy_backups"))
    args = parser.parse_args()

    args.output_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(args.output_dir, 0o700)
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    output = args.output_dir / f"tracker-{stamp}.sqlite3"
    fd = os.open(output, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    os.close(fd)
    try:
        source_uri = args.source.resolve().as_uri() + "?mode=ro"
        with sqlite3.connect(source_uri, uri=True) as source:
            with sqlite3.connect(output) as target:
                source.backup(target)
                if target.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
                    raise RuntimeError("Backup failed SQLite integrity check")
                products = target.execute("SELECT COUNT(*) FROM products").fetchone()[0]
                alerts = target.execute("SELECT COUNT(*) FROM price_alerts").fetchone()[0]
    except Exception:
        output.unlink(missing_ok=True)
        raise

    print(f"Online backup: {output} (products={products}, alerts={alerts})")


if __name__ == "__main__":
    main()
