#!/usr/bin/env bash
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="${1:-$REPO_ROOT/build/recovery-bundle}"
mkdir -p "$OUTPUT_DIR"
OUTPUT_DIR="$(cd "$OUTPUT_DIR" && pwd)"
cd "$REPO_ROOT"
export GOCACHE="${GOCACHE:-$REPO_ROOT/.gocache}"
export GOMODCACHE="${GOMODCACHE:-$REPO_ROOT/.gomodcache}"
VERSION="$(sed -n 's/^const Version = "\([^"]*\)"/\1/p' internal/buildinfo/version.go)"
RESTIC_SOURCE="$(go list -m -f '{{.Dir}}' github.com/restic/restic)"
RESTIC_VERSION="$(go list -m -f '{{.Version}}' github.com/restic/restic)"
for arch in amd64 arm64; do
  mkdir -p "$OUTPUT_DIR/linux-$arch"
  for tool in weazlback weazlback-restore; do
    GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' -o "$OUTPUT_DIR/linux-$arch/$tool" "./cmd/$tool"
  done
  (cd "$RESTIC_SOURCE" && GOOS=linux GOARCH="$arch" CGO_ENABLED=0 GOWORK=off go build -mod=readonly -buildvcs=false -trimpath -ldflags="-s -w -X main.version=${RESTIC_VERSION#v}" -o "$OUTPUT_DIR/linux-$arch/restic" ./cmd/restic)
 done
for asset in scripts/install.sh scripts/launch.sh scripts/uninstall.sh scripts/weazlback-session systemd/weazlback-backup.service systemd/weazlback-backup.timer manifest.json BarWidget.qml widget/weazlback/Model.js widget/weazlback/BarWidget.qml widget/weazlback/Panel.qml widget/weazlback/Lane.qml; do
  install -D -m0644 "$REPO_ROOT/$asset" "$OUTPUT_DIR/$asset"
done
python3 - "$OUTPUT_DIR" "$VERSION" "$RESTIC_VERSION" <<'PY'
import hashlib,json,pathlib,sys
root=pathlib.Path(sys.argv[1])
files={str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(root.rglob('*')) if p.is_file() and p.name!='BUNDLE.json'}
(root/'BUNDLE.json').write_text(json.dumps(dict(version=sys.argv[2],restic_version=sys.argv[3],files=files),indent=2)+'\n')
PY
printf 'Universal recovery bundle: %s\n' "$OUTPUT_DIR"
