#!/usr/bin/env bash
set -euo pipefail

window=${1:-48 hours ago}
if journalctl -k --since "$window" --no-pager -o cat | grep -Ei 'out of memory|oom-kill|killed process' | grep -Ei 'ahsanly|node|chrome|postgres|systemd-oomd'; then
  echo 'OOM event found in the requested window' >&2
  exit 1
fi
echo "No matching OOM event since $window"
