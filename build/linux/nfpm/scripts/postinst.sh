#!/bin/sh
# After install or upgrade. deb: "configure" [old version]; rpm: 1 = install,
# 2 or more = upgrade. GHOSTLINE_TEST_ROOT prefixes paths in tests only.
set -e
R=${GHOSTLINE_TEST_ROOT:-}
case "$1" in
configure) [ -n "${2:-}" ] && mode=upgrade || mode=install ;;
1) mode=install ;;
[2-9]*) mode=upgrade ;;
*) exit 0 ;; # abort-upgrade, abort-remove…: nothing to start
esac
# deb: reinstalling after "apt remove" looks like an upgrade (dpkg passes
# the old version), but the removal disabled the service; postrm left this.
if [ -e "$R/var/lib/ghostline/.package-removed" ]; then
	rm -f "$R/var/lib/ghostline/.package-removed"
	mode=install
fi
systemd-sysusers ghostline.conf || true
# A self-install (AppImage, tar.gz) put a unit in /etc that would shadow
# this package's and keep the old daemon running: retire it. The engine
# files under /var/lib/ghostline/bin stay; the package's daemon uses them.
self="$R/etc/systemd/system/ghostline.service"
if [ -f "$self" ] && grep -q '^ExecStart=/var/lib/ghostline/bin/ghostlined ' "$self"; then
	systemctl disable --now ghostline.service || true
	rm -f "$self" "$R/var/lib/ghostline/bin/ghostlined"
	mode=install
fi
systemctl daemon-reload || true
if [ "$mode" = upgrade ]; then
	systemctl try-restart ghostline.service || true
else
	systemctl enable --now ghostline.service || true
fi
