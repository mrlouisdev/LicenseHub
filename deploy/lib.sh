#!/usr/bin/env bash

# Shared local Docker Compose helpers for the checked deployment bundle.

LICENSEHUB_DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LICENSEHUB_COMPOSE_FILE="${COMPOSE_FILE:-$LICENSEHUB_DEPLOY_DIR/docker-compose.yml}"
LICENSEHUB_ENV_FILE="${ENV_FILE:-$LICENSEHUB_DEPLOY_DIR/.env}"

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
}

licensehub_compose() {
  docker compose --env-file "$LICENSEHUB_ENV_FILE" -f "$LICENSEHUB_COMPOSE_FILE" "$@"
}
