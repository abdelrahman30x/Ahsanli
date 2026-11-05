#!/usr/bin/env bash
set -euo pipefail
umask 077

month=$(date -u +%Y-%m)
directory=/etc/ahsanly
target="$directory/dbip-country-lite.mmdb"
marker="$directory/dbip-country-lite.month"
if test -f "$target" && test "$(cat "$marker" 2>/dev/null || true)" = "$month"; then exit 0; fi

install -d -m 0755 "$directory"
archive=$(mktemp "$directory/.geoip-XXXXXXXX.gz")
database=$(mktemp "$directory/.geoip-XXXXXXXX.mmdb")
trap 'rm -f "$archive" "$database"' EXIT
curl --fail --location --silent --show-error --max-time 120 \
  "https://download.db-ip.com/free/dbip-country-lite-$month.mmdb.gz" -o "$archive"
gzip -t "$archive"
gzip -dc "$archive" > "$database"
/srv/ahsanly/go_platform/bin/ahsanly-geoipverify "$database"
chmod 0644 "$database"
mv "$database" "$target"
printf '%s\n' "$month" > "$marker"
if systemctl is-active --quiet ahsanly-api.service; then systemctl restart ahsanly-api.service; fi
