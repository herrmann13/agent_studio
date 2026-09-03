#!/usr/bin/env bash
set -euo pipefail

BUN="${BUN:-bun}"
WAILS="${WAILS:-$(go env GOPATH)/bin/wails}"

command -v go >/dev/null || { echo "Go is required."; exit 1; }
command -v "$BUN" >/dev/null || { echo "Bun is required."; exit 1; }
[[ -x "$WAILS" ]] || { echo "Wails was not found at $WAILS."; exit 1; }

"$BUN" --version
go version
"$WAILS" version
"$BUN" install --cwd frontend --frozen-lockfile
go mod download
