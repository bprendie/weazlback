#!/usr/bin/env bash
set -euo pipefail

unset NO_COLOR
session_bin=${1:-${WEAZLBACK_SESSION_BIN:-"$HOME/.weazlback/bin/weazlback-session"}}

focus_client() {
  local client_pid terminal_pid address
  while read -r client_pid; do
    terminal_pid=$(ps -o ppid= -p "$client_pid" | tr -d ' ')
    [[ -n $terminal_pid ]] || continue
    address=$(hyprctl clients -j | jq -r --argjson pid "$terminal_pid" \
      '.[] | select(.pid == $pid) | .address' | head -n 1)
    [[ -n $address ]] || continue
    hyprctl dispatch "hl.dsp.focus({ window = \"address:$address\" })" >/dev/null
    return 0
  done < <(tmux list-clients -t weazlback -F '#{client_pid}' 2>/dev/null)
  return 1
}

attached=$(tmux display-message -p -t weazlback '#{session_attached}' 2>/dev/null || printf 0)
if [[ $attached != 0 ]] && focus_client; then
  exit 0
fi

omarchy-launch-tui --app-id=io.github.bprendie.weazlback \
  "$session_bin" open >/dev/null 2>&1 &

for _ in {1..30}; do
  attached=$(tmux display-message -p -t weazlback '#{session_attached}' 2>/dev/null || printf 0)
  if [[ $attached != 0 ]] && focus_client; then
    exit 0
  fi
  sleep 0.1
done
exit 1
