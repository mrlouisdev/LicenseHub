#!/usr/bin/env bash
set -Eeuo pipefail

root="${FAKE_DOCKER_ROOT:?FAKE_DOCKER_ROOT is required}"
mkdir -p "$root/container"
printf '%s\n' "$*" >>"$root/calls.log"

[[ "${1:-}" == "compose" ]] || { echo "fake docker only supports compose" >&2; exit 2; }
shift
while (($#)); do
  case "$1" in
    --env-file|-f|--project-name) shift 2 ;;
    *) break ;;
  esac
done

command="${1:-}"
shift || true
case "$command" in
  version)
    echo 'Docker Compose version v2.30.0'
    ;;
  config)
    case "${1:-}" in
      --images) echo 'postgres:15@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' ;;
      --services) echo server ;;
      *) ;;
    esac
    ;;
  up|stop)
    ;;
  cp)
    source_path="${1:?missing cp source}"
    destination="${2:?missing cp destination}"
    if [[ "$source_path" == postgres:* ]]; then
      cp "$root/container/${source_path#postgres:}" "$destination"
    else
      mkdir -p "$root/container/$(dirname "${destination#postgres:}")"
      cp "$source_path" "$root/container/${destination#postgres:}"
    fi
    ;;
  exec)
    while [[ "${1:-}" == -* ]]; do shift; done
    service="${1:?missing service}"
    shift
    [[ "$service" == postgres ]] || exit 0
    if [[ "${1:-}" == "sh" && "${2:-}" == "-c" ]]; then
      script="${3:-}"
      target="${5:-}"
      if [[ "$script" == *pg_dump* ]]; then
        cp "$root/db-state" "$root/container/$target"
      elif [[ "$script" == *pg_restore* ]]; then
        cp "$root/container/$target" "$root/db-state"
      fi
      exit 0
    fi
    case "${1:-}" in
      pg_restore)
        last="${!#}"
        [[ "${2:-}" == "--list" ]] || cp "$root/container/$last" "$root/db-state"
        ;;
      rm)
        for path in "${@:3}"; do rm -f "$root/container/$path"; done
        ;;
      pg_dump)
        ;;
    esac
    ;;
  *)
    echo "unsupported fake compose command: $command" >&2
    exit 2
    ;;
esac
