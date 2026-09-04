#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-dev}"
WAILS="${WAILS:-$(go env GOPATH)/bin/wails}"
MACHINE_ARCH="$(uname -m)"

case "$MACHINE_ARCH" in
  arm64) ARCH="arm64" ;;
  x86_64) ARCH="amd64" ;;
  *) echo "Unsupported macOS architecture: $MACHINE_ARCH"; exit 1 ;;
esac

VERSION="$VERSION" WAILS="$WAILS" WAILS_TAGS="${WAILS_TAGS:-}" bash scripts/build.sh
mkdir -p dist
staging_dir="$(mktemp -d)"
trap 'rm -rf "$staging_dir"' EXIT
cp -R "build/bin/agent-studio.app" "$staging_dir/agent-studio.app"
ln -s /Applications "$staging_dir/Applications"
hdiutil create -volname "Agent Studio" -srcfolder "$staging_dir" -ov -format UDZO "dist/agent-studio-${VERSION}-macos-${ARCH}.dmg"
