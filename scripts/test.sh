#!/usr/bin/env bash
set -euo pipefail

BUN="${BUN:-bun}"

"$BUN" install --cwd frontend --frozen-lockfile
"$BUN" run --cwd frontend build
go test ./...
