#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-dev}"
WAILS="${WAILS:-$(go env GOPATH)/bin/wails}"

[[ -x "$WAILS" ]] || { echo "Wails was not found at $WAILS."; exit 1; }
go mod verify

if [[ -n "${WAILS_TAGS:-}" ]]; then
  read -r -a WAILS_TAGS <<< "$WAILS_TAGS"
  "$WAILS" build "${WAILS_TAGS[@]}" -clean -trimpath -ldflags "-X main.version=$VERSION"
else
  "$WAILS" build -clean -trimpath -ldflags "-X main.version=$VERSION"
fi
