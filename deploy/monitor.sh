#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"
licensehub_require_runtime

domain="$(licensehub_env_get LICENSE_DOMAIN)"
[[ "$domain" =~ ^[a-z0-9.-]+$ ]] || { echo "invalid LICENSE_DOMAIN" >&2; exit 1; }
state_dir="${LICENSEHUB_OPS_STATE_DIR:-$SCRIPT_DIR/../ops-state}"
state_file="$state_dir/monitor.status"
umask 077
mkdir -p "$state_dir"
previous="unknown"
[[ -f "$state_file" ]] && previous="$(cat "$state_file")"

failure=""
if ! output="$($SCRIPT_DIR/verify.sh "https://$domain" 2>&1)"; then
  failure="public acceptance failed: ${output//$'\n'/ }"
elif ! licensehub_compose exec -T server curl -fsS http://127.0.0.1:9000/health >/dev/null; then
  failure="internal health probe failed"
fi

if [[ -n "$failure" ]]; then
  printf 'failed\n' >"$state_file"
  [[ "$previous" == "failed" ]] || licensehub_alert failed "$failure"
  echo "MONITOR_FAIL $failure" >&2
  exit 1
fi
printf 'healthy\n' >"$state_file"
[[ "$previous" == "healthy" ]] || licensehub_alert healthy "LicenseHub recovered and all production probes pass"
echo "MONITOR_OK https://$domain"
