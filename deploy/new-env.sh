#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  echo "Usage: $0 <license-domain> <admin-email> [output-file]" >&2
  exit 2
}

[[ $# -ge 2 && $# -le 3 ]] || usage
domain="${1,,}"
admin_email="${2,,}"
output="${3:-$(cd "$(dirname "$0")" && pwd)/.env}"

[[ "$domain" =~ ^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$ && "$domain" == *.* ]] || {
  echo "invalid license domain" >&2
  exit 2
}
[[ "$admin_email" =~ ^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,63}$ ]] || {
  echo "invalid admin email" >&2
  exit 2
}
[[ ! -e "$output" ]] || {
  echo "refusing to overwrite existing environment file: $output" >&2
  exit 1
}
command -v openssl >/dev/null 2>&1 || {
  echo "openssl is required" >&2
  exit 1
}

umask 077
mkdir -p "$(dirname "$output")"
tmp="$(mktemp "${output}.tmp.XXXXXX")"
trap 'rm -f "$tmp"' EXIT

postgres_password="$(openssl rand -hex 32)"
jwt_secret="$(openssl rand -hex 32)"
license_signing_key="$(openssl rand -hex 32)"
release_encryption_key="$(openssl rand -hex 32)"
metrics_token="$(openssl rand -hex 32)"

unique_count="$(printf '%s\n' \
  "$postgres_password" "$jwt_secret" "$license_signing_key" \
  "$release_encryption_key" "$metrics_token" | sort -u | wc -l | tr -d ' ')"
[[ "$unique_count" == "5" ]] || {
  echo "random secret collision; retry" >&2
  exit 1
}

cat >"$tmp" <<EOF
LICENSE_DOMAIN=$domain
ACME_EMAIL=$admin_email
LICENSEHUB_VERSION=${LICENSEHUB_VERSION:-0.1.0}

POSTGRES_USER=licensehub
POSTGRES_DB=licensehub
POSTGRES_PASSWORD=$postgres_password
DATABASE_URL=postgres://licensehub:$postgres_password@postgres:5432/licensehub?sslmode=disable

JWT_SECRET=$jwt_secret
LICENSE_SIGNING_KEY=$license_signing_key
LICENSE_SIGNING_KEY_ID=v1
LICENSE_LEASE_TTL=72h
LICENSE_RETAINED_PUBLIC_KEYS={}
RELEASE_KEY_ENCRYPTION_KEY=$release_encryption_key
METRICS_TOKEN=$metrics_token
ADMIN_EMAILS=$admin_email

RATE_LIMIT_API=60
RATE_LIMIT_ADMIN=120
RATE_LIMIT_AUTH=20
EOF

chmod 600 "$tmp"
mv "$tmp" "$output"
trap - EXIT
echo "ENV_READY $output"
