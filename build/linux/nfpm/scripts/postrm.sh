#!/bin/sh
# After removal. deb "purge" also removes the settings and the logs; deb
# "remove" leaves a marker so a reinstall starts the service again.
# GHOSTLINE_TEST_ROOT prefixes paths in tests only.
set -e
R=${GHOSTLINE_TEST_ROOT:-}
case "$1" in
purge) rm -rf "$R/var/lib/ghostline" "$R/var/log/ghostline" ;;
remove)
	mkdir -p "$R/var/lib/ghostline"
	: > "$R/var/lib/ghostline/.package-removed"
	systemctl daemon-reload 2>/dev/null || true
	;;
0) systemctl daemon-reload 2>/dev/null || true ;;
esac
