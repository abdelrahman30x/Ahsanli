#!/usr/bin/env bash
set -euo pipefail
backup_dir=/root/botAfflite/data/pg_backups
latest=$(find "$backup_dir" -maxdepth 1 -type f -name 'botafflite-*.dump' | sort | tail -n 1)
test -n "$latest"
cd "$backup_dir"
sha256sum -c "$(basename "$latest").sha256"
test_db="botafflite_restore_$(date -u +%s)"
cleanup() { runuser -u postgres -- dropdb --if-exists "$test_db" >/dev/null; }
trap cleanup EXIT
runuser -u postgres -- createdb "$test_db"
runuser -u postgres -- pg_restore --no-owner --no-acl --dbname "$test_db" < "$latest"
runuser -u postgres -- psql --dbname "$test_db" --tuples-only --no-align --command 'SELECT COUNT(*) FROM core_listing' >/dev/null
echo "PostgreSQL restore test passed"
