#!/bin/sh
# Removes what install.sh installed. The system (DNS, proxy, certificates,
# firewall) is restored first. --purge also removes the settings and logs
# in /var/lib/ghostline and /var/log/ghostline.
set -eu
[ "$(id -u)" -eq 0 ] || { echo "Run as root: sudo ./uninstall.sh [--purge]" >&2; exit 1; }
PREFIX=${PREFIX:-/usr/local}
purge=
case "${1:-}" in
--purge) purge=--purge ;;
"") ;;
*) echo "usage: uninstall.sh [--purge]" >&2; exit 2 ;;
esac
for d in /var/lib/ghostline/bin/ghostlined "$PREFIX/lib/ghostline/ghostlined"; do
	if [ -x "$d" ]; then
		"$d" --uninstall-system $purge
		break
	fi
done
rm -f "$PREFIX/bin/ghostline" "$PREFIX/lib/ghostline/ghostlined" \
	"$PREFIX/share/applications/ghostline.desktop" \
	"$PREFIX/share/icons/hicolor/512x512/apps/ghostline.png" \
	"$PREFIX/share/metainfo/io.github.hashcott.ghostline.metainfo.xml"
rmdir "$PREFIX/lib/ghostline" 2>/dev/null || true
command -v update-desktop-database >/dev/null && update-desktop-database -q "$PREFIX/share/applications" || true
echo "Ghostline is removed."
