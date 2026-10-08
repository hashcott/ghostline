#!/bin/sh
# Before removal (not before an upgrade: deb "upgrade", rpm 1). The service
# stops first (it disconnects and its ExecStopPost restores the system),
# then --remove-certs takes away anything left: certificates, firewall
# rules, the nftables table.
set -e
GHOSTLINED=${GHOSTLINED:-/usr/lib/ghostline/ghostlined}
case "$1" in
remove | 0) ;;
*) exit 0 ;;
esac
systemctl disable --now ghostline.service || true
"$GHOSTLINED" --remove-certs || true
