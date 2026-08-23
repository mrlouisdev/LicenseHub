#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

[[ $# -ge 2 && $# -le 3 ]] || {
  echo "Usage: $0 <backup-directory> <age-identity-file> [environment-output]" >&2
  exit 2
}
backup="$(cd "$1" 2>/dev/null && pwd)" || { echo "backup directory not found" >&2; exit 1; }
identity="$(realpath "$2" 2>/dev/null)" || { echo "age identity file not found" >&2; exit 1; }
output="$(realpath -m "${3:-$SCRIPT_DIR/.env}")"

licensehub_require_private_file "$identity"
command -v age >/dev/null 2>&1 || { echo "age is required" >&2; exit 1; }
[[ ! -e "$output" ]] || { echo "refusing to overwrite existing environment" >&2; exit 1; }
for file in database.dump recovery.env.age image-lock.txt backup-manifest.json checksums.sha256; do
  [[ -f "$backup/$file" ]] || { echo "missing backup file: $file" >&2; exit 1; }
done
(cd "$backup" && sha256sum --check checksums.sha256)

umask 077
mkdir -p "$(dirname "$output")"
tmp="$(mktemp "${output}.tmp.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
age --decrypt --identity "$identity" --output "$tmp" "$backup/recovery.env.age"
[[ -s "$tmp" ]] || { echo "decrypted environment is empty" >&2; exit 1; }
chmod 600 "$tmp"
LICENSEHUB_ENV_FILE="$tmp" LICENSEHUB_COMPOSE_FILE="${LICENSEHUB_COMPOSE_FILE:-$SCRIPT_DIR/docker-compose.yml}" \
  bash -c 'source "$1/lib.sh"; licensehub_require_runtime; licensehub_compose config --quiet' bash "$SCRIPT_DIR"

expected_images="$(LC_ALL=C sort "$backup/image-lock.txt")"
actual_images="$(LICENSEHUB_ENV_FILE="$tmp" LICENSEHUB_COMPOSE_FILE="${LICENSEHUB_COMPOSE_FILE:-$SCRIPT_DIR/docker-compose.yml}" \
  bash -c 'source "$1/lib.sh"; licensehub_compose config --images' bash "$SCRIPT_DIR" | LC_ALL=C sort -u)"
[[ "$actual_images" == "$expected_images" ]] || {
  echo "destination image set does not match the backup image lock" >&2
  exit 1
}
mv "$tmp" "$output"
trap - EXIT
LICENSEHUB_ENV_FILE="$output" "$SCRIPT_DIR/restore.sh" "$backup" --force
echo "CLEAN_HOST_RECOVERY_COMPLETE $output"
