#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  echo "Usage: $0 <admin-email> [admin-name] [product-name] [product-slug] [desktop|saas|hybrid]" >&2
  exit 2
}

[[ $# -ge 1 && $# -le 5 ]] || usage
admin_email="${1,,}"
admin_name="${2:-LicenseHub Owner}"
product_name="${3:-LicenseHub Test Product}"
product_slug="${4:-licensehub-test}"
product_type="${5:-desktop}"

[[ "$admin_email" =~ ^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,63}$ ]] || {
  echo "invalid bootstrap admin email" >&2
  exit 2
}
simple_name_re='^[[:alnum:]][[:alnum:] ._-]{0,79}$'
for value in "$admin_name" "$product_name"; do
  [[ "$value" =~ $simple_name_re ]] || {
    echo "bootstrap names must be 1-80 simple characters" >&2
    exit 2
  }
done
[[ "$product_slug" =~ ^[a-z0-9][a-z0-9-]{0,62}$ ]] || {
  echo "invalid product slug" >&2
  exit 2
}
[[ "$product_type" =~ ^(desktop|saas|hybrid)$ ]] || {
  echo "invalid product type" >&2
  exit 2
}

licensehub_require_runtime
licensehub_compose config --quiet
caddy_present=0
if licensehub_compose config --services | grep -Fxq caddy; then
  caddy_present=1
fi

# Keep the one-time unauthenticated setup endpoint off the public edge.
if [[ "$caddy_present" == "1" ]]; then
  licensehub_compose stop caddy >/dev/null 2>&1 || true
fi
licensehub_compose up -d --build postgres server

healthy=0
for _ in {1..60}; do
  if licensehub_compose exec -T server curl -fsS http://127.0.0.1:9000/health >/dev/null 2>&1; then
    healthy=1
    break
  fi
  sleep 2
done
[[ "$healthy" == "1" ]] || {
  licensehub_compose logs --tail 100 server >&2 || true
  echo "server failed to become healthy" >&2
  exit 1
}

status_body="$(licensehub_compose exec -T server curl -fsS \
  http://127.0.0.1:9000/api/v1/setup/status)"
if grep -Eq '"needed"[[:space:]]*:[[:space:]]*false' <<<"$status_body"; then
  echo "SETUP_ALREADY_COMPLETE"
else
  payload="$(printf \
    '{"admin_email":"%s","admin_name":"%s","site_name":"LicenseHub","product_name":"%s","product_slug":"%s","product_type":"%s"}' \
    "$admin_email" "$admin_name" "$product_name" "$product_slug" "$product_type")"
  response="$(licensehub_compose exec -T server curl --silent --show-error \
    --header 'Content-Type: application/json' --data "$payload" \
    --write-out $'\n%{http_code}' http://127.0.0.1:9000/api/v1/setup/initialize)"
  http_status="${response##*$'\n'}"
  body="${response%$'\n'*}"
  [[ "$http_status" == "201" ]] || {
    echo "bootstrap initialize returned HTTP $http_status: $body" >&2
    exit 1
  }
  echo "SETUP_INITIALIZED"
fi

if [[ "$caddy_present" == "1" ]]; then
  licensehub_compose up -d caddy
fi
echo "BOOTSTRAP_COMPLETE"
