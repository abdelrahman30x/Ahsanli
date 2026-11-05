#!/usr/bin/env bash
set -euo pipefail
umask 022

repo=${AHSANLY_REPO:-abdelrahman30x/Ahsanli}
srv=/srv/ahsanly
state_dir=/var/lib/ahsanly/deploy
release_root="$srv/releases"
current_file="$state_dir/current-revision"
lock_file=/run/ahsanly-deploy.lock

install -d -m 0755 "$state_dir" "$release_root"
exec 9>"$lock_file"
flock -n 9 || exit 0

work=$(mktemp -d /var/lib/ahsanly/.deploy.XXXXXXXX)
trap 'rm -rf "$work"' EXIT

fetch() {
  curl --fail --location --silent --show-error \
    --connect-timeout 10 --max-time 120 --retry 2 --retry-delay 3 "$@"
}

fetch "https://api.github.com/repos/${repo}/releases/latest" -o "$work/release.json"
manifest_url=$(jq -er '.assets[] | select(.name == "manifest.json") | .browser_download_url' "$work/release.json")
bundle_url=$(jq -er '.assets[] | select(.name == "ahsanly-linux-amd64.tar.gz") | .browser_download_url' "$work/release.json")
fetch "$manifest_url" -o "$work/manifest.json"

revision=$(jq -er '.revision | select(test("^[0-9a-f]{40}$"))' "$work/manifest.json")
expected_sha=$(jq -er '.bundle_sha256 | select(test("^[0-9a-f]{64}$"))' "$work/manifest.json")
if [[ -f "$current_file" ]] && [[ "$(cat "$current_file")" == "$revision" ]]; then
  exit 0
fi

fetch "$bundle_url" -o "$work/bundle.tar.gz"
printf '%s  %s\n' "$expected_sha" "$work/bundle.tar.gz" | sha256sum --check --status

mkdir "$work/payload"
while IFS= read -r entry; do
  case "$entry" in
    /*|..|../*|*/../*|*/..|.|./*|*/./*|*/.) echo "Unsafe release archive entry" >&2; exit 1 ;;
  esac
done < <(tar -tzf "$work/bundle.tar.gz")
tar -xzf "$work/bundle.tar.gz" -C "$work/payload" --no-same-owner --no-same-permissions

for binary in api migrate pipeline worker; do
  test -f "$work/payload/go_platform/bin/ahsanly-$binary"
  chmod 0755 "$work/payload/go_platform/bin/ahsanly-$binary"
done
test -f "$work/payload/web_next/server.js"

/bin/bash "$srv/ops/ahsanly-backup.sh"
set -a
. /etc/ahsanly/ahsanly.env
set +a
"$work/payload/go_platform/bin/ahsanly-migrate" --apply

destination="$release_root/$revision"
if [[ -e "$destination" ]]; then
  echo "Release directory already exists but is not active: $revision" >&2
  exit 1
fi
mv "$work/payload" "$destination"

previous_go=$(readlink -f "$srv/go_platform" || true)
previous_web=$(readlink -f "$srv/web_next" || true)
if [[ -z "$previous_go" || -z "$previous_web" ]]; then
  echo "Current application paths are missing" >&2
  exit 1
fi

legacy="$srv/pre-autodeploy"
if [[ ! -L "$srv/go_platform" ]]; then
  install -d -m 0755 "$legacy"
  mv "$srv/go_platform" "$legacy/go_platform"
fi
if [[ ! -L "$srv/web_next" ]]; then
  install -d -m 0755 "$legacy"
  mv "$srv/web_next" "$legacy/web_next"
fi
case "$previous_go" in
  "$release_root"/*) previous_root=$(dirname "$previous_go") ;;
  *) previous_root="$legacy" ;;
esac

switch_to() {
  local target=$1
  ln -sfn "$target/go_platform" "$srv/go_platform.next"
  mv -Tf "$srv/go_platform.next" "$srv/go_platform"
  ln -sfn "$target/web_next" "$srv/web_next.next"
  mv -Tf "$srv/web_next.next" "$srv/web_next"
}

restart_services() {
  systemctl restart ahsanly-api.service ahsanly-web.service ahsanly-worker.service ahsanly-pipeline.service
}

switch_to "$destination"
if ! restart_services; then
  switch_to "$previous_root"
  restart_services || true
  exit 1
fi

sleep 4
healthy=1
for unit in ahsanly-api ahsanly-web ahsanly-worker ahsanly-pipeline; do
  systemctl is-active --quiet "$unit.service" || healthy=0
done
curl --fail --silent --max-time 10 http://127.0.0.1:8083/api/v1/health >/dev/null || healthy=0
curl --fail --silent --max-time 15 http://127.0.0.1:3100/ar >/dev/null || healthy=0

if [[ "$healthy" -ne 1 ]]; then
  switch_to "$previous_root"
  restart_services || true
  echo "Deployment health check failed; restored the previous code" >&2
  exit 1
fi

printf '%s\n' "$revision" > "$state_dir/current-revision.next"
mv -f "$state_dir/current-revision.next" "$current_file"
echo "AHSANLY deployment active: $revision"
