#!/usr/bin/env bash
set -euo pipefail

APP_NAME="weazlback"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_ROOT="${WEAZLBACK_HOME:-"$HOME/.weazlback"}"
BIN_DIR="$INSTALL_ROOT/bin"
SYSTEMD_USER_DIR="${XDG_CONFIG_HOME:-"$HOME/.config"}/systemd/user"
PLUGIN_ID="io.github.bprendie.weazlback"
PLUGIN_DIR="${XDG_CONFIG_HOME:-"$HOME/.config"}/omarchy/plugins/$PLUGIN_ID"
GO_CACHE="${GOCACHE:-"$REPO_ROOT/.gocache"}"
GO_MOD_CACHE="${GOMODCACHE:-"$REPO_ROOT/.gomodcache"}"

mkdir -p "$BIN_DIR" "$GO_CACHE" "$GO_MOD_CACHE"
chmod 0700 "$INSTALL_ROOT" "$BIN_DIR"
mkdir -p "${XDG_CONFIG_HOME:-"$HOME/.config"}/weazlback" "${XDG_CACHE_HOME:-"$HOME/.cache"}/weazlback"
chmod 0700 "${XDG_CONFIG_HOME:-"$HOME/.config"}/weazlback" "${XDG_CACHE_HOME:-"$HOME/.cache"}/weazlback"

build_binary() {
  local name="$1"
  local package="$2"
  local temporary
  temporary="$(mktemp "$BIN_DIR/.${name}.XXXXXX")"
  if ! (
    cd "$REPO_ROOT"
    CGO_ENABLED=0 GOCACHE="$GO_CACHE" GOMODCACHE="$GO_MOD_CACHE" \
      go build -buildvcs=false -trimpath -o "$temporary" "$package"
  ); then
    rm -f "$temporary"
    return 1
  fi
  chmod 0755 "$temporary"
  mv -f "$temporary" "$BIN_DIR/$name"
}

install_binary() {
  local name="$1"
  local temporary
  temporary="$(mktemp "$BIN_DIR/.${name}.XXXXXX")"
  install -m0755 "$REPO_ROOT/$name" "$temporary"
  mv -f "$temporary" "$BIN_DIR/$name"
}

if [[ -d "$REPO_ROOT/cmd/weazlback" ]]; then
  command -v go >/dev/null 2>&1 || {
    echo "Go is required to build Weazlback from source." >&2
    exit 1
  }
  echo "Building Weazlback..."
  build_binary "weazlback" "./cmd/weazlback"
  build_binary "weazlback-restore" "./cmd/weazlback-restore"
else
  echo "Installing prebuilt Weazlback binaries..."
  install_binary "weazlback"
  install_binary "weazlback-restore"
fi
install -m0755 "$REPO_ROOT/scripts/weazlback-session" "$BIN_DIR/weazlback-session"
install -m0755 "$REPO_ROOT/scripts/launch.sh" "$BIN_DIR/weazlback-launch"
install -m0755 "$REPO_ROOT/scripts/uninstall.sh" "$BIN_DIR/weazlback-uninstall"

install_widget() {
  command -v omarchy >/dev/null 2>&1 || return
  mkdir -p "$PLUGIN_DIR/widget/weazlback"
  install -m0644 "$REPO_ROOT/manifest.json" "$PLUGIN_DIR/manifest.json"
  install -m0644 "$REPO_ROOT/BarWidget.qml" "$PLUGIN_DIR/BarWidget.qml"
  install -m0644 "$REPO_ROOT/widget/weazlback/Model.js" "$PLUGIN_DIR/widget/weazlback/Model.js"
  install -m0644 "$REPO_ROOT/widget/weazlback/BarWidget.qml" "$PLUGIN_DIR/widget/weazlback/BarWidget.qml"
  install -m0644 "$REPO_ROOT/widget/weazlback/Panel.qml" "$PLUGIN_DIR/widget/weazlback/Panel.qml"
  install -m0644 "$REPO_ROOT/widget/weazlback/Lane.qml" "$PLUGIN_DIR/widget/weazlback/Lane.qml"
  omarchy plugin validate "$PLUGIN_DIR"
  if [[ "${WEAZLBACK_SKIP_WIDGET_ACTIVATE:-0}" != "1" ]]; then
    omarchy-shell shell rescanPlugins
    omarchy plugin enable "$PLUGIN_ID"
    omarchy bar move "$PLUGIN_ID" --section right --before omarchy.audio
    omarchy bar set "$PLUGIN_ID" binaryPath "$BIN_DIR/weazlback"
  fi
  echo "Installed Omarchy widget to $PLUGIN_DIR"
}

install_widget

install_schedule() {
	[[ "${WEAZLBACK_SKIP_SCHEDULE:-0}" == "1" ]] && return
  command -v systemctl >/dev/null 2>&1 || return
  mkdir -p "$SYSTEMD_USER_DIR"
  install -m0644 "$REPO_ROOT/systemd/weazlback-backup.service" "$SYSTEMD_USER_DIR/weazlback-backup.service"
  install -m0644 "$REPO_ROOT/systemd/weazlback-backup.timer" "$SYSTEMD_USER_DIR/weazlback-backup.timer"
  systemctl --user daemon-reload
  systemctl --user enable --now weazlback-backup.timer
  echo "Installed hourly overdue-backup timer"
}

install_schedule

profile="$HOME/.profile"
if [[ "${SHELL:-}" == */bash && -f "$HOME/.bashrc" ]]; then
  profile="$HOME/.bashrc"
elif [[ "${SHELL:-}" == */zsh ]]; then
  profile="$HOME/.zshrc"
fi

marker_begin="# >>> weazlback path >>>"
marker_end="# <<< weazlback path <<<"
path_line='export PATH="$HOME/.weazlback/bin:$PATH"'
if [[ ":$PATH:" != *":$BIN_DIR:"* ]] && ! grep -Fq "$marker_begin" "$profile" 2>/dev/null; then
  {
    echo
    echo "$marker_begin"
    echo "$path_line"
    echo "$marker_end"
  } >> "$profile"
fi

echo "Installed binaries to $BIN_DIR"
echo "Installed tmux session backend and Omarchy Quattro companion widget."

RECOVERY_MEDIA="/mnt/WEAZLBACK-RECOVERY"
if [[ "${WEAZLBACK_SKIP_RECOVERY_REFRESH:-0}" != "1" && -d "$RECOVERY_MEDIA" && -w "$RECOVERY_MEDIA" ]]; then
  "$BIN_DIR/weazlback" recovery refresh --target "$RECOVERY_MEDIA"
fi

if [[ "${WEAZLBACK_SKIP_LAUNCH:-0}" != "1" ]]; then
  exec "$BIN_DIR/$APP_NAME"
fi
