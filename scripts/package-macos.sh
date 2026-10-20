#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-dev}"
WAILS="${WAILS:-$(go env GOPATH)/bin/wails}"
MACHINE_ARCH="$(uname -m)"

case "$MACHINE_ARCH" in
  arm64) ARCH="arm64"; ARCH_LABEL="Apple Silicon" ;;
  x86_64) ARCH="amd64"; ARCH_LABEL="Intel" ;;
  *) echo "Unsupported macOS architecture: $MACHINE_ARCH"; exit 1 ;;
esac

VERSION="$VERSION" WAILS="$WAILS" WAILS_TAGS="${WAILS_TAGS:-}" bash scripts/build.sh
mkdir -p dist
staging_dir="$(mktemp -d)"
trap 'rm -rf "$staging_dir"' EXIT
cp -R "build/bin/agent-studio.app" "$staging_dir/agent-studio.app"
ln -s /Applications "$staging_dir/Applications"
# The file name keeps the `-macos-<arch>.dmg` suffix the in-app updater matches;
# the volume name tells the two builds apart once the DMG is opened.
hdiutil create -volname "Agent Studio (${ARCH_LABEL})" -srcfolder "$staging_dir" -ov -format UDZO "dist/agent-studio-${VERSION}-macos-${ARCH}.dmg"
