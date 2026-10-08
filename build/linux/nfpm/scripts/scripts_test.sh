#!/bin/bash
# Tests the package scripts with stub systemctl, systemd-sysusers and
# ghostlined that log their calls. Run: bash build/linux/nfpm/scripts/scripts_test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
fail=0
setup() {
	tmp=$(mktemp -d)
	mkdir -p "$tmp/bin" "$tmp/root/var/lib/ghostline/data" "$tmp/root/var/log/ghostline"
	for c in systemctl systemd-sysusers ghostlined; do
		printf '#!/bin/sh\necho "%s $*" >> "%s/log"\n' "$c" "$tmp" > "$tmp/bin/$c"
		chmod +x "$tmp/bin/$c"
	done
	: > "$tmp/log"
}
run() { # run <script> <args…>
	PATH="$tmp/bin:$PATH" GHOSTLINED="$tmp/bin/ghostlined" ROOT="$tmp/root" sh "$here/$1" "${@:2}"
}
expect() { # expect <name> <expected log>
	got=$(cat "$tmp/log")
	if [ "$got" != "$2" ]; then
		echo "FAIL $1"; echo "  want: $(echo "$2" | tr '\n' '|')"; echo "  got:  $(echo "$got" | tr '\n' '|')"; fail=1
	else
		echo "ok   $1"
	fi
	rm -rf "$tmp"
}
fresh=$'systemd-sysusers ghostline.conf\nsystemctl daemon-reload\nsystemctl enable --now ghostline.service'
upgrade=$'systemd-sysusers ghostline.conf\nsystemctl daemon-reload\nsystemctl try-restart ghostline.service'
removal=$'systemctl disable --now ghostline.service\nghostlined --remove-certs'

setup; run postinst.sh configure; expect "deb install" "$fresh"
setup; run postinst.sh configure 0.6.0; expect "deb upgrade" "$upgrade"
setup; run postinst.sh 1; expect "rpm install" "$fresh"
setup; run postinst.sh 2; expect "rpm upgrade" "$upgrade"
setup; run postinst.sh abort-upgrade 0.6.1; expect "deb abort-upgrade" ""
# Review Focus 1: an upgrade keeps the system as it is.
setup; run prerm.sh upgrade 0.6.1; expect "deb prerm upgrade" ""
setup; run prerm.sh 1; expect "rpm prerm upgrade" ""
setup; run prerm.sh remove; expect "deb prerm remove" "$removal"
setup; run prerm.sh 0; expect "rpm prerm remove" "$removal"
setup; run postrm.sh purge
if [ -e "$tmp/root/var/lib/ghostline" ] || [ -e "$tmp/root/var/log/ghostline" ]; then echo "FAIL deb purge keeps data"; fail=1; else echo "ok   deb purge removes data"; fi
rm -rf "$tmp"
setup; run postrm.sh remove
if [ -e "$tmp/root/var/lib/ghostline/data" ]; then echo "ok   deb remove keeps data"; else echo "FAIL deb remove dropped data"; fail=1; fi
rm -rf "$tmp"
exit $fail
