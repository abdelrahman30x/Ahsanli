# 9. GitHub deployment, server updates, and rollback

## Release pipeline

Routine production deployment starts with a push to `main` in `abdelrahman30x/Ahsanli`. `.github/workflows/build-release.yml` uses a production concurrency group and performs:

1. Repository checkout.
2. Documentation freshness check (`python docs/build_docs.py --check`).
3. Go setup from `go.mod` and `go test ./...`.
4. Linux AMD64, CGO-disabled, trimmed builds for `api`, `migrate`, `worker`, and `pipeline`.
5. Node 24 setup, `npm ci`, and `npm run build`.
6. Next standalone packaging, including `.next/static` and `public`.
7. One `ahsanly-linux-amd64.tar.gz` bundle plus SHA-256 `manifest.json` containing full commit and build time.
8. A `deploy-<full-commit>` GitHub release marked latest.

A failed test/build prevents release publication. Release assets do not contain environment files, database dumps, Nginx configuration, captures, or Telegram credentials.

## VPS automatic installer

`ahsanly-deploy.timer` runs `ahsanly-deploy.service` every two minutes with a small randomized delay. The service executes `/usr/local/sbin/ahsanly-deploy`, installed from `ops/ahsanly-deploy.sh`.

For each new latest release it:

1. Uses a nonblocking `flock` to prevent overlapping installers.
2. Downloads release metadata, manifest, and bundle over HTTPS.
3. Validates full commit shape and SHA-256 before extraction.
4. Rejects unsafe archive paths and verifies all application binaries and `web_next/server.js`.
5. Runs `ops/ahsanly-backup.sh`, which writes and validates a PostgreSQL custom-format backup.
6. Loads the protected environment without printing it and applies embedded migrations.
7. Moves the payload to `/srv/ahsanly/releases/<commit>`.
8. Atomically switches the stable Go and Next symlinks.
9. Restarts exactly `ahsanly-api`, `ahsanly-web`, `ahsanly-worker`, and `ahsanly-pipeline`.
10. Checks all units, local `/api/v1/health`, and local `/ar`.
11. On failure, restores the previous code symlinks and restarts the previous code without restoring old data.
12. Writes `/var/lib/ahsanly/deploy/current-revision` only after successful health checks.

If the latest manifest already equals the active revision, the installer exits without backup/restart.

## Normal operator commands

```bash
# Active source revision and timer
cat /var/lib/ahsanly/deploy/current-revision
systemctl list-timers ahsanly-deploy.timer

# Logs and immediate poll
journalctl -u ahsanly-deploy.service -n 100 --no-pager
systemctl start ahsanly-deploy.service

# Pause/resume automatic deployment during maintenance
systemctl disable --now ahsanly-deploy.timer
systemctl enable --now ahsanly-deploy.timer

# Application health
systemctl is-active ahsanly-api ahsanly-web ahsanly-worker ahsanly-pipeline
systemctl show ahsanly-api ahsanly-web ahsanly-worker ahsanly-pipeline \
  -p MemoryCurrent -p MemoryPeak -p NRestarts
```

## Manual code rollback

Disable the timer first if the newer latest release should not automatically return. Select an existing known-good full commit under `/srv/ahsanly/releases`:

```bash
revision=<full-commit>
test -x /srv/ahsanly/releases/$revision/go_platform/bin/ahsanly-api
test -f /srv/ahsanly/releases/$revision/web_next/server.js

ln -sfn /srv/ahsanly/releases/$revision/go_platform /srv/ahsanly/go_platform.next
mv -Tf /srv/ahsanly/go_platform.next /srv/ahsanly/go_platform
ln -sfn /srv/ahsanly/releases/$revision/web_next /srv/ahsanly/web_next.next
mv -Tf /srv/ahsanly/web_next.next /srv/ahsanly/web_next

systemctl restart ahsanly-api ahsanly-web ahsanly-worker ahsanly-pipeline
printf '%s\n' "$revision" > /var/lib/ahsanly/deploy/current-revision
```

Run local health checks afterward. Code rollback does not reverse schema migrations and must not discard newer observations. This is why migrations should remain compatible with the previous release.

## Database backups

`ops/ahsanly-backup.sh` uses `pg_dump` custom format, validates the archive with `pg_restore --list`, writes SHA-256, and prunes old dumps after the retention period. Daily/restore-test timers also exist. Backups currently have no assumed off-server destination; do not claim off-site disaster recovery unless separately configured and verified.

## Historical cutover

The BotAfflite-to-AHSANLY database cutover was a one-time operation: stop writers, take a final consistent snapshot, restore into `ahsanly`, preserve IDs, apply migrations, then start only the new units. Those commands are retained in `AGENTS.md` and `ops/SERVER.md` for history. They are not the routine deployment process.

Never restore the pre-cutover `botafflite` dump over `ahsanly` unless intentionally discarding all post-cutover data; that is not a valid ordinary rollback.
