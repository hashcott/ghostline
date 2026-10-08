#!/bin/sh
# After removal. deb "purge" also removes the settings and the logs.
set -e
case "$1" in
purge) rm -rf "${ROOT:-}/var/lib/ghostline" "${ROOT:-}/var/log/ghostline" ;;
remove | 0) systemctl daemon-reload 2>/dev/null || true ;;
esac
