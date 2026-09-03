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

VERSION="$VERSION" WAILS="$WAILS" bash scripts/build.sh
mkdir -p dist
hdiutil create -volname "Agent Studio" -srcfolder "build/bin/agent-studio.app" -ov -format UDZO "dist/agent-studio-${VERSION}-macos-${ARCH}.dmg"
