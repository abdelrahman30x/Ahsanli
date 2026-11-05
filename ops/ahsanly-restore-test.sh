#!/usr/bin/env bash
set -euo pipefail

backup_dir=/var/backups/ahsanly/pg
latest=$(find "$backup_dir" -maxdepth 1 -type f -name 'ahsanly-*.dump' -printf '%T@ %p\n' | sort -nr | head -1 | cut -d' ' -f2-)
test -n "$latest"
sha256sum --check "$latest.sha256"

test_database=ahsanly_restore_test
cleanup() { runuser -u postgres -- dropdb --if-exists "$test_database" >/dev/null; }
trap cleanup EXIT
cleanup
runuser -u postgres -- createdb "$test_database"
cat "$latest" | runuser -u postgres -- pg_restore --no-owner --no-privileges --single-transaction -d "$test_database"
runuser -u postgres -- psql -v ON_ERROR_STOP=1 -At -d "$test_database" -c 'SELECT count(*) FROM core_listing;'
echo "Restore test passed: $latest"
