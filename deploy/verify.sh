#!/usr/bin/env bash
set -Eeuo pipefail

[[ $# -eq 1 ]] || {
  echo "Usage: $0 https://license.example.com" >&2
  exit 2
}
base_url="${1%/}"
[[ "$base_url" == https://* ]] || {
  echo "production verification requires an https URL" >&2
  exit 2
}
command -v curl >/dev/null 2>&1 || {
  echo "curl is required" >&2
  exit 1
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

request() {
  local name="$1"
  shift
  curl --silent --show-error --connect-timeout 10 --max-time 30 \
    --dump-header "$tmp/$name.headers" --output "$tmp/$name.body" \
    --write-out '%{http_code}' "$@"
}

status="$(request health "$base_url/health")"
[[ "$status" == "200" ]] || { echo "health returned HTTP $status" >&2; exit 1; }
grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' "$tmp/health.body" || {
  echo "health body is not ok" >&2
  exit 1
}

for expected in \
  'strict-transport-security:[[:space:]]*max-age=' \
  'x-content-type-options:[[:space:]]*nosniff' \
  'x-frame-options:[[:space:]]*DENY' \
  'referrer-policy:'; do
  tr -d '\r' <"$tmp/health.headers" | grep -Eqi "^$expected" || {
    echo "missing security header: $expected" >&2
    exit 1
  }
done
if tr -d '\r' <"$tmp/health.headers" | grep -Eqi '^server:'; then
  echo "edge leaked Server header" >&2
  exit 1
fi

status="$(request metrics "$base_url/metrics")"
[[ "$status" == "404" ]] || { echo "public metrics returned HTTP $status, expected 404" >&2; exit 1; }

status="$(request keys "$base_url/v1/client/public-keys")"
[[ "$status" == "200" ]] || { echo "public keys returned HTTP $status" >&2; exit 1; }
grep -Eq '"keys"[[:space:]]*:' "$tmp/keys.body" || {
  echo "public key ring is missing" >&2
  exit 1
}

status="$(request strict-json \
  --header 'Content-Type: application/json' \
  --data '{"license_key":"invalid","product_id":"invalid","device_id":"probe","unexpected":true}' \
  "$base_url/v1/client/activate")"
[[ "$status" == "400" ]] || {
  echo "strict JSON probe returned HTTP $status, expected 400" >&2
  exit 1
}

echo "PUBLIC_ACCEPTANCE_OK $base_url"
