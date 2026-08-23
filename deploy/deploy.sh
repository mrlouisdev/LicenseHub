#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"
licensehub_require_runtime
licensehub_compose config --quiet

mode="$(stat -c '%a' "$LICENSEHUB_ENV_FILE")"
[[ "$mode" == "600" ]] || {
  echo "environment file must be mode 600: $LICENSEHUB_ENV_FILE (found $mode)" >&2
  exit 1
}

existing_postgres="$(licensehub_compose ps -a -q postgres 2>/dev/null || true)"
backup_path=""
if [[ -n "$existing_postgres" ]]; then
  backup_output="$($SCRIPT_DIR/backup.sh "$SCRIPT_DIR/../backups/pre-deploy")"
  echo "$backup_output"
  backup_path="$(printf '%s\n' "$backup_output" | awk '/^BACKUP_READY /{sub(/^BACKUP_READY /, ""); p=$0} END{print p}')"
  [[ -n "$backup_path" ]] || {
    echo "pre-deploy backup did not return an artifact path" >&2
    exit 1
  }
fi

licensehub_compose build --pull server
licensehub_compose up -d postgres redis server

if ! licensehub_wait_for_health server; then
  licensehub_compose logs --tail 100 server >&2 || true
  echo "server failed to become healthy; restore with: ./restore.sh <backup> --force" >&2
  exit 1
fi

setup_status="$(licensehub_compose exec -T server curl -fsS \
  http://127.0.0.1:9000/api/v1/setup/status)"
if grep -Eq '"needed"[[:space:]]*:[[:space:]]*true' <<<"$setup_status"; then
  echo "initial setup is incomplete; run ./bootstrap.sh before exposing Caddy" >&2
  exit 1
fi

if licensehub_manage_caddy; then
  licensehub_compose up -d caddy
else
  echo "CADDY_EXTERNAL skipped embedded Caddy"
fi
echo "DEPLOY_COMPLETE${backup_path:+ BACKUP $backup_path}"
