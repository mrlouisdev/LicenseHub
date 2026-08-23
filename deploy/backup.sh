#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"
licensehub_require_runtime
command -v age >/dev/null 2>&1 || { echo "age is required for encrypted recovery backups" >&2; exit 1; }

age_recipient="$(awk -F= '$1 == "BACKUP_AGE_RECIPIENT" {sub(/^[^=]*=/, ""); print; exit}' "$LICENSEHUB_ENV_FILE")"
[[ "$age_recipient" =~ ^age1[023456789acdefghjklmnpqrstuvwxyz]{58}$ ]] || {
  echo "BACKUP_AGE_RECIPIENT must be a valid age X25519 public recipient" >&2
  exit 1
}

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

# The encrypted recovery bundle is sufficient to reconstruct database access,
# JWT/session signing, offline-license signing, release encryption, SMTP and
# optional storage credentials on a clean host. Plaintext never enters the
# backup directory and the environment file is passed as a file, not argv.
age --encrypt --recipient "$age_recipient" \
  --output "$output/recovery.env.age" "$LICENSEHUB_ENV_FILE"
[[ -s "$output/recovery.env.age" ]] || { echo "encrypted recovery bundle is empty" >&2; exit 1; }

licensehub_compose config --images | LC_ALL=C sort -u >"$output/image-lock.txt"
[[ -s "$output/image-lock.txt" ]] || { echo "image lock metadata is empty" >&2; exit 1; }

cat >"$output/backup-manifest.json" <<EOF
{
  "format_version": 2,
  "created_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "encryption": "age-x25519",
  "recovery_bundle": "recovery.env.age",
  "files": ["database.dump", "recovery.env.age", "image-lock.txt"]
}
EOF

(
  cd "$output"
  sha256sum database.dump recovery.env.age image-lock.txt backup-manifest.json >checksums.sha256
  sha256sum --check checksums.sha256 >/dev/null
)
chmod 600 "$output"/*

retention_days="$(awk -F= '$1 == "BACKUP_RETENTION_DAYS" {print $2; exit}' "$LICENSEHUB_ENV_FILE")"
retention_days="${retention_days:-30}"
[[ "$retention_days" =~ ^[1-9][0-9]{0,3}$ ]] || { echo "invalid BACKUP_RETENTION_DAYS" >&2; exit 1; }
find "$(realpath "$output_root")" -mindepth 1 -maxdepth 1 -type d \
  -regextype posix-extended -regex '.*/[0-9]{8}T[0-9]{6}Z' \
  -mtime "+$retention_days" -exec rm -rf -- {} +

offsite_hook="$(awk -F= '$1 == "BACKUP_OFFSITE_HOOK" {sub(/^[^=]*=/, ""); print; exit}' "$LICENSEHUB_ENV_FILE")"
if [[ -n "$offsite_hook" ]]; then
  licensehub_require_secure_hook "$offsite_hook"
  # Only the encrypted artifact directory is made available. The hook reads
  # credentials from its own protected configuration and must upload atomically.
  BACKUP_PATH="$output" "$offsite_hook"
  touch "$output/offsite-upload.ok"
  chmod 600 "$output/offsite-upload.ok"
fi
licensehub_alert healthy "encrypted LicenseHub recovery backup completed: $stamp"
echo "BACKUP_READY $output"
