#!/bin/sh
# After install or upgrade. deb: "configure" [old version]; rpm: 1 = install,
# 2 or more = upgrade.
set -e
case "$1" in
configure) [ -n "${2:-}" ] && mode=upgrade || mode=install ;;
1) mode=install ;;
[2-9]*) mode=upgrade ;;
*) exit 0 ;; # abort-upgrade, abort-remove…: nothing to start
esac
systemd-sysusers ghostline.conf || true
systemctl daemon-reload || true
if [ "$mode" = upgrade ]; then
	systemctl try-restart ghostline.service || true
else
	systemctl enable --now ghostline.service || true
fi
