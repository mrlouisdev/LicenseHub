#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  echo "Usage: $0 <backup-directory> --force" >&2
  exit 2
}

[[ $# -eq 2 && "$2" == "--force" ]] || usage
backup="$(cd "$1" 2>/dev/null && pwd)" || {
  echo "backup directory not found: $1" >&2
  exit 1
}
licensehub_require_runtime
for file in database.dump recovery.env.age image-lock.txt backup-manifest.json checksums.sha256; do
  [[ -f "$backup/$file" ]] || { echo "missing backup file: $file" >&2; exit 1; }
done
checksum_entries="$(awk 'NF == 2 && $1 ~ /^[0-9a-fA-F]{64}$/ { print $2 }' "$backup/checksums.sha256" | sort)"
[[ "$checksum_entries" == $'backup-manifest.json\ndatabase.dump\nimage-lock.txt\nrecovery.env.age' ]] || {
  echo "checksum manifest must contain exactly the v2 backup artifacts" >&2
  exit 1
}
(
  cd "$backup"
  sha256sum --check checksums.sha256
)

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
safety_dir="$SCRIPT_DIR/../backups/pre-restore-safety/$stamp"
restore_dump="/tmp/licensehub-restore-$$.dump"
safety_dump="/tmp/licensehub-safety-$$.dump"
safety_ready=0
completed=0

managed_services=(server)
if licensehub_manage_caddy; then
  managed_services+=(caddy)
fi

cleanup() {
  licensehub_compose exec -T postgres rm -f "$restore_dump" "$safety_dump" >/dev/null 2>&1 || true
}

on_exit() {
  local status=$?
  trap - EXIT ERR
  set +e
  if [[ "$status" != "0" && "$completed" == "0" && "$safety_ready" == "1" ]]; then
    echo "restore failed; rolling database back to the pre-restore safety dump" >&2
    if ! licensehub_compose exec -T postgres sh -c \
      'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges --single-transaction --exit-on-error "$1"' \
      sh "$safety_dump"; then
      echo "CRITICAL: pre-restore safety rollback failed; manual recovery required" >&2
      cleanup
      exit 70
    fi
  fi
  if [[ "$status" == "0" || "$safety_ready" == "1" ]]; then
    licensehub_compose up -d "${managed_services[@]}" >/dev/null 2>&1 || true
  fi
  cleanup
  exit "$status"
}
trap on_exit EXIT

licensehub_compose up -d postgres
licensehub_compose cp "$backup/database.dump" "postgres:$restore_dump" >/dev/null
licensehub_compose exec -T postgres pg_restore --list "$restore_dump" >/dev/null

licensehub_compose stop "${managed_services[@]}"
umask 077
mkdir -p "$safety_dir"
licensehub_compose exec -T postgres sh -c \
  'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f "$1"' sh "$safety_dump"
licensehub_compose cp "postgres:$safety_dump" "$safety_dir/database.dump" >/dev/null
[[ -s "$safety_dir/database.dump" ]] || { echo "safety backup is empty" >&2; exit 1; }
cat >"$safety_dir/backup-manifest.json" <<EOF
{
  "format_version": 1,
  "created_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "reason": "pre-restore safety backup",
  "files": ["database.dump"]
}
EOF
(
  cd "$safety_dir"
  sha256sum database.dump backup-manifest.json >checksums.sha256
)
safety_ready=1

licensehub_compose exec -T postgres sh -c \
  'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner --no-privileges --single-transaction --exit-on-error "$1"' \
  sh "$restore_dump"
licensehub_compose up -d server
if ! licensehub_wait_for_health server; then
  licensehub_compose logs --tail 100 server >&2 || true
  echo "restored server did not become healthy" >&2
  exit 1
fi
if [[ " ${managed_services[*]} " == *" caddy "* ]]; then
  licensehub_compose up -d caddy
fi
completed=1
echo "RESTORE_COMPLETE $backup SAFETY_BACKUP $safety_dir"
