#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-dev}"
WAILS="${WAILS:-$(go env GOPATH)/bin/wails}"
ARCH="$(dpkg --print-architecture)"
APPIMAGETOOL="${APPIMAGETOOL:-}"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

case "$ARCH" in
  amd64) TOOL_ARCH="x86_64" ;;
  arm64) TOOL_ARCH="aarch64" ;;
  *) TOOL_ARCH="$ARCH" ;;
esac

command -v mksquashfs >/dev/null || { echo "mksquashfs (squashfs-tools) is required to build an AppImage."; exit 1; }

VERSION="$VERSION" WAILS="$WAILS" WAILS_TAGS="${WAILS_TAGS:-}" bash scripts/build.sh

APPDIR="$WORK/agent-studio.AppDir"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/256x256/apps"
install -m 0755 build/bin/agent-studio "$APPDIR/usr/bin/agent-studio"
install -m 0644 build/appicon.png "$APPDIR/agent-studio.png"
install -m 0644 build/appicon.png "$APPDIR/usr/share/icons/hicolor/256x256/apps/agent-studio.png"

cat > "$APPDIR/agent-studio.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=Agent Studio
Comment=Manage skills for terminal coding agents
Exec=agent-studio
Icon=agent-studio
Categories=Development;
Terminal=false
EOF

install -m 0644 "$APPDIR/agent-studio.desktop" "$APPDIR/usr/share/applications/agent-studio.desktop"

cat > "$APPDIR/AppRun" <<'EOF'
#!/bin/sh
SELF=$(readlink -f "$0")
HERE=${SELF%/*}
export PATH="$HERE/usr/bin:$PATH"
exec "$HERE/usr/bin/agent-studio" "$@"
EOF
chmod +x "$APPDIR/AppRun"

if [[ -z "$APPIMAGETOOL" ]]; then
  APPIMAGETOOL="$(command -v appimagetool || true)"
fi
if [[ -z "$APPIMAGETOOL" ]]; then
  curl -fL -o "$WORK/appimagetool" "https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-${TOOL_ARCH}.AppImage"
  chmod +x "$WORK/appimagetool"
  APPIMAGETOOL="$WORK/appimagetool"
fi

mkdir -p dist
APPIMAGE_EXTRACT_AND_RUN=1 "$APPIMAGETOOL" "$APPDIR" "dist/agent-studio-${VERSION}-linux-${ARCH}.AppImage"
