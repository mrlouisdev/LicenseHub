#!/usr/bin/env bash

# Shared local Docker Compose helpers for the checked deployment bundle.

LICENSEHUB_DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LICENSEHUB_COMPOSE_FILE="${LICENSEHUB_COMPOSE_FILE:-${COMPOSE_FILE:-$LICENSEHUB_DEPLOY_DIR/docker-compose.yml}}"
LICENSEHUB_ENV_FILE="${LICENSEHUB_ENV_FILE:-${ENV_FILE:-$LICENSEHUB_DEPLOY_DIR/.env}}"
LICENSEHUB_SECRETS_DIR="${LICENSEHUB_SECRETS_DIR:-$(dirname "$LICENSEHUB_ENV_FILE")/secrets}"
export LICENSEHUB_COMPOSE_FILE LICENSEHUB_ENV_FILE LICENSEHUB_SECRETS_DIR

licensehub_require_runtime() {
  command -v docker >/dev/null 2>&1 || {
    echo "docker is required" >&2
    return 1
  }
  docker compose version >/dev/null 2>&1 || {
    echo "docker compose v2 is required" >&2
    return 1
  }
  [[ -f "$LICENSEHUB_COMPOSE_FILE" ]] || {
    echo "compose file not found: $LICENSEHUB_COMPOSE_FILE" >&2
    return 1
  }
  [[ -f "$LICENSEHUB_ENV_FILE" ]] || {
    echo "environment file not found: $LICENSEHUB_ENV_FILE" >&2
    return 1
  }
  licensehub_require_private_file "$LICENSEHUB_ENV_FILE"
  licensehub_materialize_secrets
  licensehub_validate_image_refs
}

licensehub_compose() {
  docker compose --env-file "$LICENSEHUB_ENV_FILE" -f "$LICENSEHUB_COMPOSE_FILE" "$@"
}

licensehub_require_private_file() {
  local file="$1" mode
  [[ -f "$file" ]] || { echo "file not found: $file" >&2; return 1; }
  mode="$(stat -c '%a' "$file")"
  [[ "$mode" == "600" || "$mode" == "400" ]] || {
    echo "restricted file must be mode 600 or 400: $file (found $mode)" >&2
    return 1
  }
}

licensehub_env_get() {
  local key="$1"
  awk -v key="$key" '
    index($0, key "=") == 1 { sub(/^[^=]*=/, ""); print; found=1; exit }
    END { if (!found) exit 1 }
  ' "$LICENSEHUB_ENV_FILE"
}

licensehub_materialize_secret() {
  local name="$1" key="$2" owner="$3" value tmp
  value="$(licensehub_env_get "$key" 2>/dev/null || true)"
  tmp="$(mktemp "$LICENSEHUB_SECRETS_DIR/.${name}.XXXXXX")"
  printf '%s' "$value" >"$tmp"
  chmod 0400 "$tmp"
  if [[ "$(id -u)" == "0" ]]; then
    chown "$owner:$owner" "$tmp"
  fi
  mv -f "$tmp" "$LICENSEHUB_SECRETS_DIR/$name"
}

licensehub_materialize_secrets() {
  local app_uid="${LICENSEHUB_APP_UID:-10001}"
  local postgres_uid="${LICENSEHUB_POSTGRES_UID:-70}"
  [[ "$app_uid" =~ ^[0-9]+$ && "$postgres_uid" =~ ^[0-9]+$ ]] || {
    echo "secret file UIDs must be numeric" >&2
    return 1
  }
  umask 077
  mkdir -p "$LICENSEHUB_SECRETS_DIR"
  LICENSEHUB_SECRETS_DIR="$(cd "$LICENSEHUB_SECRETS_DIR" && pwd -P)"
  export LICENSEHUB_SECRETS_DIR
  chmod 0700 "$LICENSEHUB_SECRETS_DIR"

  licensehub_materialize_secret postgres_password POSTGRES_PASSWORD "$postgres_uid"
  licensehub_materialize_secret database_url DATABASE_URL "$app_uid"
  licensehub_materialize_secret jwt_secret JWT_SECRET "$app_uid"
  licensehub_materialize_secret license_signing_key LICENSE_SIGNING_KEY "$app_uid"
  licensehub_materialize_secret release_key_encryption_key RELEASE_KEY_ENCRYPTION_KEY "$app_uid"
  licensehub_materialize_secret metrics_token METRICS_TOKEN "$app_uid"
  licensehub_materialize_secret smtp_password SMTP_PASSWORD "$app_uid"
  licensehub_materialize_secret storage_access_key STORAGE_ACCESS_KEY "$app_uid"
  licensehub_materialize_secret storage_secret_key STORAGE_SECRET_KEY "$app_uid"
}

licensehub_require_secure_hook() {
  local hook="$1" owner mode
  [[ "$hook" == /* && -f "$hook" && -x "$hook" && ! -L "$hook" ]] || {
    echo "hook must be an absolute executable regular file" >&2
    return 1
  }
  owner="$(stat -c '%u' "$hook")"
  mode="$(stat -c '%a' "$hook")"
  [[ "$owner" == "0" || "$owner" == "$(id -u)" ]] || {
    echo "hook has an untrusted owner" >&2
    return 1
  }
  (( (8#$mode & 022) == 0 )) || {
    echo "hook must not be group/world writable" >&2
    return 1
  }
}

licensehub_service_defined() {
  licensehub_compose config --services 2>/dev/null | grep -Fxq "$1"
}

licensehub_manage_caddy() {
  local requested="${LICENSEHUB_MANAGE_CADDY:-auto}"
  case "$requested" in
    1|true|yes|embedded) licensehub_service_defined caddy ;;
    0|false|no|external) return 1 ;;
    auto) licensehub_service_defined caddy ;;
    *) echo "invalid LICENSEHUB_MANAGE_CADDY: $requested" >&2; return 2 ;;
  esac
}

licensehub_require_digest_ref() {
  local name="$1" value="$2"
  [[ "$value" =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || {
    echo "$name must be an immutable image reference ending in @sha256:<64 lowercase hex>" >&2
    return 1
  }
}

licensehub_validate_image_refs() {
  local ref
  while IFS= read -r ref; do
    [[ -n "$ref" ]] || continue
    # Locally built application images are identified by immutable image ID
    # after build. Every image pulled from a registry must be digest pinned.
    case "$ref" in
      licensehub-server:*|licensehub-bootstrap:*) continue ;;
    esac
    licensehub_require_digest_ref "compose image" "$ref"
  done < <(licensehub_compose config --images)
}

licensehub_wait_for_health() {
  local service="${1:-server}" attempts="${LICENSEHUB_HEALTH_ATTEMPTS:-60}" i
  [[ "$attempts" =~ ^[1-9][0-9]*$ ]] || { echo "invalid LICENSEHUB_HEALTH_ATTEMPTS" >&2; return 2; }
  [[ "${LICENSEHUB_TEST_FORCE_HEALTH_FAILURE:-0}" != "1" ]] || return 1
  for ((i=0; i<attempts; i++)); do
    if licensehub_compose exec -T "$service" curl -fsS http://127.0.0.1:9000/health >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

licensehub_json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  printf '%s' "$value"
}

licensehub_alert() {
  local status="$1" message="$2" hook
  hook="$(licensehub_env_get ALERT_HOOK 2>/dev/null || true)"
  [[ -n "$hook" ]] || return 0
  licensehub_require_secure_hook "$hook"
  LICENSEHUB_ALERT_STATUS="$status" \
  LICENSEHUB_ALERT_MESSAGE="$message" \
  LICENSEHUB_ALERT_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    "$hook"
}
