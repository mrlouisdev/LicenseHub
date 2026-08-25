#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cp "$repo_root/deploy/lib.sh" "$tmp/lib.sh"
touch "$tmp/compose.yml"
cat >"$tmp/runtime.env" <<'EOF'
POSTGRES_PASSWORD=db-fixture
DATABASE_URL=postgres://fixture
JWT_SECRET=jwt-fixture
LICENSE_SIGNING_KEY=signing-fixture
RELEASE_KEY_ENCRYPTION_KEY=encryption-fixture
METRICS_TOKEN=metrics-fixture
SMTP_PASSWORD=smtp-fixture
STORAGE_ACCESS_KEY=storage-access-fixture
STORAGE_SECRET_KEY=storage-secret-fixture
EOF
chmod 0600 "$tmp/runtime.env"

LICENSEHUB_ENV_FILE="$tmp/runtime.env"
LICENSEHUB_COMPOSE_FILE="$tmp/compose.yml"
LICENSEHUB_SECRETS_DIR="$tmp/runtime-secrets"
LICENSEHUB_APP_UID="$(id -u)"
LICENSEHUB_POSTGRES_UID="$(id -u)"
export LICENSEHUB_ENV_FILE LICENSEHUB_COMPOSE_FILE LICENSEHUB_SECRETS_DIR
export LICENSEHUB_APP_UID LICENSEHUB_POSTGRES_UID
# shellcheck source=../../deploy/lib.sh
source "$tmp/lib.sh"
licensehub_materialize_secrets

[[ "$(stat -c '%a' "$LICENSEHUB_SECRETS_DIR")" == 700 ]]
expected=(postgres_password database_url jwt_secret license_signing_key release_key_encryption_key metrics_token smtp_password storage_access_key storage_secret_key)
for name in "${expected[@]}"; do
  [[ -f "$LICENSEHUB_SECRETS_DIR/$name" ]]
  [[ "$(stat -c '%a' "$LICENSEHUB_SECRETS_DIR/$name")" == 400 ]]
done
[[ "$(<"$LICENSEHUB_SECRETS_DIR/smtp_password")" == smtp-fixture ]]

# A second run replaces files atomically and leaves no staging files behind.
sed -i 's/smtp-fixture/smtp-rotated/' "$LICENSEHUB_ENV_FILE"
licensehub_materialize_secrets
[[ "$(<"$LICENSEHUB_SECRETS_DIR/smtp_password")" == smtp-rotated ]]
if find "$LICENSEHUB_SECRETS_DIR" -maxdepth 1 -name '.*.??????' | grep -q .; then
  echo 'temporary secret file was not cleaned up' >&2
  exit 1
fi

echo 'PASS file-backed secret materialization'
