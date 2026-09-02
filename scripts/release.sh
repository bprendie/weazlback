#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$(sed -n 's/^const Version = "\([^"]*\)"/\1/p' "$REPO_ROOT/internal/app/run.go")"
OUTPUT_DIR="${1:-$REPO_ROOT/dist}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

mkdir -p "$OUTPUT_DIR" "$STAGE/weazlback-$VERSION"
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' -o "$STAGE/weazlback-$VERSION/weazlback" "$REPO_ROOT/cmd/weazlback"
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' -o "$STAGE/weazlback-$VERSION/weazlback-restore" "$REPO_ROOT/cmd/weazlback-restore"
install -m0644 "$REPO_ROOT/README.md" "$STAGE/weazlback-$VERSION/README.md"
install -m0644 "$REPO_ROOT/LICENSE" "$STAGE/weazlback-$VERSION/LICENSE"
install -m0644 "$REPO_ROOT/THIRD_PARTY_NOTICES.md" "$STAGE/weazlback-$VERSION/THIRD_PARTY_NOTICES.md"
install -m0644 "$REPO_ROOT/manifest.json" "$STAGE/weazlback-$VERSION/manifest.json"
install -D -m0755 "$REPO_ROOT/scripts/install.sh" "$STAGE/weazlback-$VERSION/scripts/install.sh"
install -D -m0755 "$REPO_ROOT/scripts/uninstall.sh" "$STAGE/weazlback-$VERSION/scripts/uninstall.sh"
install -D -m0755 "$REPO_ROOT/scripts/launch.sh" "$STAGE/weazlback-$VERSION/scripts/launch.sh"
install -D -m0755 "$REPO_ROOT/scripts/weazlback-session" "$STAGE/weazlback-$VERSION/scripts/weazlback-session"
install -D -m0644 "$REPO_ROOT/systemd/weazlback-backup.service" "$STAGE/weazlback-$VERSION/systemd/weazlback-backup.service"
install -D -m0644 "$REPO_ROOT/systemd/weazlback-backup.timer" "$STAGE/weazlback-$VERSION/systemd/weazlback-backup.timer"
install -D -m0644 "$REPO_ROOT/BarWidget.qml" "$STAGE/weazlback-$VERSION/BarWidget.qml"
for asset in Model.js BarWidget.qml Panel.qml Lane.qml; do
  install -D -m0644 "$REPO_ROOT/widget/weazlback/$asset" "$STAGE/weazlback-$VERSION/widget/weazlback/$asset"
done
install -D -m0644 "$REPO_ROOT/docs/SSH_TARGET.md" "$STAGE/weazlback-$VERSION/docs/SSH_TARGET.md"
install -D -m0644 "$REPO_ROOT/docs/RELEASE_ACCEPTANCE.md" "$STAGE/weazlback-$VERSION/docs/RELEASE_ACCEPTANCE.md"
TZ=UTC tar --sort=name --mtime='UTC 2020-01-01' --owner=0 --group=0 --numeric-owner -C "$STAGE" -czf "$OUTPUT_DIR/weazlback-$VERSION-linux-amd64.tar.gz" "weazlback-$VERSION"
(cd "$OUTPUT_DIR" && sha256sum "weazlback-$VERSION-linux-amd64.tar.gz" > "weazlback-$VERSION-linux-amd64.tar.gz.sha256")
if command -v minisign >/dev/null 2>&1 && [[ -n "${WEAZLBACK_MINISIGN_KEY:-}" ]]; then
  minisign -Sm "$OUTPUT_DIR/weazlback-$VERSION-linux-amd64.tar.gz" -s "$WEAZLBACK_MINISIGN_KEY"
fi
echo "Release artifact and checksum written to $OUTPUT_DIR"
