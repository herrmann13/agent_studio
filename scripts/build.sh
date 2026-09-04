#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-dev}"
WAILS="${WAILS:-$(go env GOPATH)/bin/wails}"
read -r -a WAILS_TAGS <<< "${WAILS_TAGS:-}"

[[ -x "$WAILS" ]] || { echo "Wails was not found at $WAILS."; exit 1; }
"$WAILS" build "${WAILS_TAGS[@]}" -clean -trimpath -ldflags "-X main.version=$VERSION"
