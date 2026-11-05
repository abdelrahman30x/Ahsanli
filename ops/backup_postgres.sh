#!/usr/bin/env bash
set -euo pipefail
umask 077
backup_dir=/root/botAfflite/data/pg_backups
mkdir -p "$backup_dir"
stamp=$(date -u +%Y%m%d-%H%M%S)
target="$backup_dir/botafflite-$stamp.dump"
partial="$target.partial"
trap 'rm -f "$partial"' EXIT
runuser -u postgres -- pg_dump --format=custom --no-owner botafflite > "$partial"
mv "$partial" "$target"
sha256sum "$target" > "$target.sha256"
find "$backup_dir" -maxdepth 1 -type f -name 'botafflite-*.dump' -mtime +14 -delete
find "$backup_dir" -maxdepth 1 -type f -name 'botafflite-*.dump.sha256' -mtime +14 -delete
echo "PostgreSQL backup completed: $target"
