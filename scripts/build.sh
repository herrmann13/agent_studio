#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-dev}"
WAILS="${WAILS:-$(go env GOPATH)/bin/wails}"

[[ -x "$WAILS" ]] || { echo "Wails was not found at $WAILS."; exit 1; }
"$WAILS" build -clean -trimpath -ldflags "-X main.version=$VERSION"
