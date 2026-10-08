#!/bin/sh
# Installs Ghostline from this folder: the window under $PREFIX
# (/usr/local) and the background service (ghostlined --install-system).
set -eu
[ "$(id -u)" -eq 0 ] || { echo "Run as root: sudo ./install.sh" >&2; exit 1; }
here=$(cd "$(dirname "$0")" && pwd)
PREFIX=${PREFIX:-/usr/local}
install -Dm755 "$here/ghostline" "$PREFIX/bin/ghostline"
# The window installs the service from here when it is missing.
install -Dm755 "$here/ghostlined" "$PREFIX/lib/ghostline/ghostlined"
install -Dm644 "$here/ghostline.desktop" "$PREFIX/share/applications/ghostline.desktop"
install -Dm644 "$here/ghostline.png" "$PREFIX/share/icons/hicolor/512x512/apps/ghostline.png"
install -Dm644 "$here/io.github.hashcott.ghostline.metainfo.xml" "$PREFIX/share/metainfo/io.github.hashcott.ghostline.metainfo.xml"
command -v update-desktop-database >/dev/null && update-desktop-database -q "$PREFIX/share/applications" || true
"$PREFIX/lib/ghostline/ghostlined" --install-system
echo "Ghostline is installed: open it from the menu. The background service is running."
