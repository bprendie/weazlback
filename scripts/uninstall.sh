#!/usr/bin/env bash
set -euo pipefail

plugin_id="io.github.bprendie.weazlback"
systemd_user_dir="${XDG_CONFIG_HOME:-"$HOME/.config"}/systemd/user"

if command -v systemctl >/dev/null 2>&1; then
  systemctl --user disable --now weazlback-backup.timer 2>/dev/null || true
  systemctl --user stop weazlback-backup.service 2>/dev/null || true
  unlink "$systemd_user_dir/weazlback-backup.timer" 2>/dev/null || true
  unlink "$systemd_user_dir/weazlback-backup.service" 2>/dev/null || true
  systemctl --user daemon-reload
fi

if command -v omarchy >/dev/null 2>&1; then
  omarchy plugin remove "$plugin_id" --yes 2>/dev/null || true
fi

echo "Removed the Weazlback timer and widget."
echo "Vaults, repositories, recovery media, logs, tombstones, and binaries were preserved."
