"""Record the current exposed-token fingerprint once, without printing it.

Run on the VPS with deal.env loaded before the user rotates the bot token.
The exclusive create refuses to replace the baseline after rotation.
"""

import json
import os
from hashlib import sha256
from pathlib import Path


root = Path("/root/botAfflite")
config = json.loads((root / "tracker_config.json").read_text(encoding="utf-8"))
site = next((row for row in config.get("sites", []) if row.get("code") == "amazon_eg"), {})
legacy = (site.get("telegram_bot_token") or config.get("telegram_bot_token") or "").strip()
go = os.environ.get("DEAL_TELEGRAM_TOKEN_KOUPNAK", "").strip()
if len(legacy) < 40 or legacy != go:
    raise SystemExit("Go and legacy token references do not match; baseline not recorded")
target = root / "data" / "pilot" / "legacy_token_before_rotation.sha256"
os.umask(0o077)
with target.open("x", encoding="ascii") as handle:
    handle.write(sha256(legacy.encode("utf-8")).hexdigest() + "\n")
target.chmod(0o600)
print("Baseline fingerprint recorded; token value not displayed")
