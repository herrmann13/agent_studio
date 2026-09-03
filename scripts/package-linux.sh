#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-dev}"
WAILS="${WAILS:-$(go env GOPATH)/bin/wails}"
ARCH="$(dpkg --print-architecture)"
PACKAGE_ROOT="$(mktemp -d)"

trap 'rm -rf "$PACKAGE_ROOT"' EXIT
command -v dpkg-deb >/dev/null || { echo "dpkg-deb is required to create a DEB package."; exit 1; }

VERSION="$VERSION" WAILS="$WAILS" bash scripts/build.sh
mkdir -p "$PACKAGE_ROOT/DEBIAN" "$PACKAGE_ROOT/usr/bin" "$PACKAGE_ROOT/usr/share/applications" "$PACKAGE_ROOT/usr/share/icons/hicolor/256x256/apps" dist
install -m 0755 build/bin/agent-studio "$PACKAGE_ROOT/usr/bin/agent-studio"
install -m 0644 build/appicon.png "$PACKAGE_ROOT/usr/share/icons/hicolor/256x256/apps/agent-studio.png"

cat > "$PACKAGE_ROOT/DEBIAN/control" <<EOF
Package: agent-studio
Version: ${VERSION#v}
Architecture: $ARCH
Maintainer: Henrique Herrmann <h.herrmann@estudantes.ifg.edu.br>
Description: Local-first skill workspace for terminal coding agents
Depends: libgtk-3-0, libwebkit2gtk-4.0-37 | libwebkit2gtk-4.1-0
EOF

cat > "$PACKAGE_ROOT/usr/share/applications/agent-studio.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Agent Studio
Comment=Manage skills for terminal coding agents
Exec=/usr/bin/agent-studio
Icon=agent-studio
Categories=Development;
Terminal=false
EOF

dpkg-deb --build --root-owner-group "$PACKAGE_ROOT" "dist/agent-studio-${VERSION}-linux-${ARCH}.deb"
