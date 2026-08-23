#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/deploy" "$tmp/backups/source" "$tmp/fake" "$tmp/bin"
cp "$repo_root/deploy/restore.sh" "$repo_root/deploy/lib.sh" "$tmp/deploy/"
cp "$repo_root/tests/deployment/fake-docker.sh" "$tmp/bin/docker"
chmod 700 "$tmp/bin/docker" "$tmp/deploy"/*.sh
touch "$tmp/deploy/docker-compose.yml"
printf 'TEST_DATABASE_URL=unused\n' >"$tmp/deploy/.env"
chmod 600 "$tmp/deploy/.env"

printf 'before\n' >"$tmp/fake/db-state"
printf 'restored\n' >"$tmp/backups/source/database.dump"
printf 'recovery\n' >"$tmp/backups/source/recovery.env.age"
printf 'postgres:15@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n' >"$tmp/backups/source/image-lock.txt"
printf '{"format_version":2}\n' >"$tmp/backups/source/backup-manifest.json"
(cd "$tmp/backups/source" && sha256sum backup-manifest.json database.dump image-lock.txt recovery.env.age >checksums.sha256)

set +e
PATH="$tmp/bin:$PATH" \
FAKE_DOCKER_ROOT="$tmp/fake" \
COMPOSE_FILE="$tmp/deploy/docker-compose.yml" \
ENV_FILE="$tmp/deploy/.env" \
LICENSEHUB_TEST_FORCE_HEALTH_FAILURE=1 \
bash "$tmp/deploy/restore.sh" "$tmp/backups/source" --force >"$tmp/stdout" 2>"$tmp/stderr"
status=$?
set -e

[[ "$status" -ne 0 ]] || { echo 'expected forced health failure to return nonzero' >&2; exit 1; }
[[ "$(tr -d '\r\n' <"$tmp/fake/db-state")" == before ]] || {
  echo 'database state did not roll back to the safety dump' >&2
  exit 1
}
grep -q 'restore failed; rolling database back' "$tmp/stderr"
[[ -s "$tmp/backups/source/database.dump" ]] || { echo 'source backup was removed' >&2; exit 1; }
restore_calls="$(grep -c 'pg_restore' "$tmp/fake/calls.log")"
[[ "$restore_calls" -ge 3 ]] || {
  echo "expected list, restore and rollback pg_restore calls; got $restore_calls" >&2
  exit 1
}
safety_dump="$(find "$tmp/backups/pre-restore-safety" -type f -name database.dump -print -quit)"
[[ -n "$safety_dump" && -s "$safety_dump" ]] || { echo 'safety dump was not retained' >&2; exit 1; }
[[ "$(tr -d '\r\n' <"$safety_dump")" == before ]] || { echo 'safety dump content mismatch' >&2; exit 1; }

echo 'PASS restore rollback failure-path test'
