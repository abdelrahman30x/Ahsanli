#!/usr/bin/env bash
set -euo pipefail
umask 077

backup_dir=/var/backups/ahsanly/pg
install -d -m 0700 "$backup_dir"
stamp=$(date -u +%Y%m%d-%H%M%S)
target="$backup_dir/ahsanly-$stamp.dump"
temporary=$(mktemp "$backup_dir/.ahsanly-XXXXXXXX.dump")
trap 'rm -f "$temporary"' EXIT

runuser -u postgres -- pg_dump --format=custom --no-owner --no-privileges ahsanly > "$temporary"
pg_restore --list "$temporary" > /dev/null
mv "$temporary" "$target"
sha256sum "$target" > "$target.sha256"
find "$backup_dir" -maxdepth 1 -type f -name 'ahsanly-*.dump' -mtime +14 -delete
find "$backup_dir" -maxdepth 1 -type f -name 'ahsanly-*.dump.sha256' -mtime +14 -delete
echo "Backup validated: $target"
