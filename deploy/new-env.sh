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

required_settings=(SMTP_HOST SMTP_USERNAME SMTP_PASSWORD SMTP_FROM BACKUP_AGE_RECIPIENT)
for key in "${required_settings[@]}"; do
  [[ -n "${!key:-}" ]] || {
    echo "$key must be exported before generating a production environment" >&2
    exit 2
  }
done
[[ "$BACKUP_AGE_RECIPIENT" =~ ^age1[023456789acdefghjklmnpqrstuvwxyz]{58}$ ]] || {
  echo "BACKUP_AGE_RECIPIENT is not a valid age X25519 public recipient" >&2
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

lock_file="$(cd "$(dirname "$0")" && pwd)/images.lock"
[[ -f "$lock_file" ]] || { echo "image lock file is missing: $lock_file" >&2; exit 1; }
while IFS='=' read -r key value; do
  [[ "$key" =~ ^[A-Z][A-Z0-9_]*$ && -n "$value" ]] || continue
  if [[ -z "${!key:-}" ]]; then
    printf -v "$key" '%s' "$value"
    export "$key"
  fi
done <"$lock_file"

image_keys=(BUN_IMAGE GOLANG_IMAGE ALPINE_IMAGE POSTGRES_IMAGE REDIS_IMAGE CADDY_IMAGE)
for key in "${image_keys[@]}"; do
  value="${!key:-}"
  [[ "$value" =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || {
    echo "$key must be exported as a registry-verified digest reference" >&2
    exit 2
  }
done

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
LICENSEHUB_COMMIT=${LICENSEHUB_COMMIT:-unknown}
LICENSEHUB_BUILD_DATE=${LICENSEHUB_BUILD_DATE:-unknown}
BUN_IMAGE=$BUN_IMAGE
GOLANG_IMAGE=$GOLANG_IMAGE
ALPINE_IMAGE=$ALPINE_IMAGE
POSTGRES_IMAGE=$POSTGRES_IMAGE
REDIS_IMAGE=$REDIS_IMAGE
CADDY_IMAGE=$CADDY_IMAGE

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
BF_MAX_FAILS=5
BF_LOCKOUT_SECONDS=30

SMTP_HOST=${SMTP_HOST:-}
SMTP_PORT=${SMTP_PORT:-587}
SMTP_USERNAME=${SMTP_USERNAME:-}
SMTP_PASSWORD=${SMTP_PASSWORD:-}
SMTP_FROM=${SMTP_FROM:-}

BACKUP_AGE_RECIPIENT=$BACKUP_AGE_RECIPIENT
BACKUP_RETENTION_DAYS=${BACKUP_RETENTION_DAYS:-30}
BACKUP_OFFSITE_HOOK=${BACKUP_OFFSITE_HOOK:-}
ALERT_HOOK=${ALERT_HOOK:-}
EOF

chmod 600 "$tmp"
mv "$tmp" "$output"
trap - EXIT
echo "ENV_READY $output"
