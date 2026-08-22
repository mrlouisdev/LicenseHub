#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"
licensehub_require_runtime

output_root="${1:-$SCRIPT_DIR/../backups}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
output="$output_root/$stamp"
container_dump="/tmp/licensehub-backup-$$.dump"

cleanup() {
  licensehub_compose exec -T postgres rm -f "$container_dump" >/dev/null 2>&1 || true
}
trap cleanup EXIT

umask 077
mkdir -p "$output"
chmod 700 "$output"

licensehub_compose up -d postgres
licensehub_compose exec -T postgres sh -c \
  'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f "$1"' sh "$container_dump"
licensehub_compose cp "postgres:$container_dump" "$output/database.dump" >/dev/null
[[ -s "$output/database.dump" ]] || {
  echo "backup dump is missing or empty" >&2
  exit 1
}

cat >"$output/backup-manifest.json" <<EOF
{
  "format_version": 1,
  "created_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "files": ["database.dump"]
}
EOF

(
  cd "$output"
  sha256sum database.dump backup-manifest.json >checksums.sha256
  sha256sum --check checksums.sha256 >/dev/null
)
chmod 600 "$output"/*
echo "BACKUP_READY $output"
