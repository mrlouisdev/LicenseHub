#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
action="${1:-install}"
[[ "$EUID" == "0" ]] || { echo "run as root" >&2; exit 1; }
command -v systemctl >/dev/null 2>&1 || { echo "systemd is required" >&2; exit 1; }

units=(licensehub-monitor.service licensehub-monitor.timer licensehub-backup.service licensehub-backup.timer)
if [[ "$action" == "remove" ]]; then
  systemctl disable --now licensehub-monitor.timer licensehub-backup.timer >/dev/null 2>&1 || true
  for unit in "${units[@]}"; do rm -f "/etc/systemd/system/$unit"; done
  systemctl daemon-reload
  echo "OPERATIONS_UNINSTALLED"
  exit 0
fi
[[ "$action" == "install" ]] || { echo "Usage: $0 [install|remove]" >&2; exit 2; }

# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"
licensehub_require_private_file "$LICENSEHUB_ENV_FILE"
for command in "$SCRIPT_DIR/monitor.sh" "$SCRIPT_DIR/backup.sh"; do
  [[ -x "$command" ]] || { echo "not executable: $command" >&2; exit 1; }
done

cat >/etc/systemd/system/licensehub-monitor.service <<EOF
[Unit]
Description=LicenseHub production acceptance monitor
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
Environment=LICENSEHUB_ENV_FILE=$LICENSEHUB_ENV_FILE
Environment=LICENSEHUB_COMPOSE_FILE=$LICENSEHUB_COMPOSE_FILE
ExecStart=$SCRIPT_DIR/monitor.sh
NoNewPrivileges=true
PrivateTmp=true
EOF

cat >/etc/systemd/system/licensehub-monitor.timer <<'EOF'
[Unit]
Description=Run LicenseHub acceptance monitor every five minutes
[Timer]
OnBootSec=2min
OnUnitActiveSec=5min
AccuracySec=30s
Persistent=true
[Install]
WantedBy=timers.target
EOF

cat >/etc/systemd/system/licensehub-backup.service <<EOF
[Unit]
Description=LicenseHub encrypted recovery backup
After=docker.service
[Service]
Type=oneshot
Environment=LICENSEHUB_ENV_FILE=$LICENSEHUB_ENV_FILE
Environment=LICENSEHUB_COMPOSE_FILE=$LICENSEHUB_COMPOSE_FILE
ExecStart=$SCRIPT_DIR/backup.sh
NoNewPrivileges=true
PrivateTmp=true
EOF

cat >/etc/systemd/system/licensehub-backup.timer <<'EOF'
[Unit]
Description=Run LicenseHub encrypted backup daily
[Timer]
OnCalendar=*-*-* 03:15:00 UTC
RandomizedDelaySec=20min
Persistent=true
[Install]
WantedBy=timers.target
EOF

chmod 644 /etc/systemd/system/licensehub-{monitor,backup}.{service,timer}
systemctl daemon-reload
systemctl enable --now licensehub-monitor.timer licensehub-backup.timer
echo "OPERATIONS_INSTALLED rollback='$SCRIPT_DIR/install-operations.sh remove'"
