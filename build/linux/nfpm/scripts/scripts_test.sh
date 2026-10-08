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
	PATH="$tmp/bin:$PATH" GHOSTLINED="$tmp/bin/ghostlined" GHOSTLINE_TEST_ROOT="$tmp/root" sh "$here/$1" "${@:2}"
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

# Review I3: dpkg keeps a removed package (it has a postrm) and reinstalls
# it with "configure <old version>"; the removal disabled the service, so
# this is a fresh start, not an upgrade.
setup; run postrm.sh remove; : > "$tmp/log"; run postinst.sh configure 0.6.0
[ -e "$tmp/root/var/lib/ghostline/.package-removed" ] && { echo "FAIL marker left"; fail=1; }
expect "deb reinstall after remove" "$fresh"

# Review I2: a self-install (AppImage, tar.gz) left a unit in /etc that
# shadows the package's: it is retired and the package's service starts.
selfunit() {
	mkdir -p "$tmp/root/etc/systemd/system" "$tmp/root/var/lib/ghostline/bin/zapret2"
	printf '[Service]\nExecStart=/var/lib/ghostline/bin/ghostlined --daemon\n' > "$tmp/root/etc/systemd/system/ghostline.service"
	: > "$tmp/root/var/lib/ghostline/bin/ghostlined"
}
retired=$'systemd-sysusers ghostline.conf\nsystemctl disable --now ghostline.service\nsystemctl daemon-reload\nsystemctl enable --now ghostline.service'
for args in "configure" "configure 0.6.0" "2"; do
	setup; selfunit; run postinst.sh $args
	if [ -e "$tmp/root/etc/systemd/system/ghostline.service" ] || [ -e "$tmp/root/var/lib/ghostline/bin/ghostlined" ]; then echo "FAIL self-install left ($args)"; fail=1; fi
	[ -d "$tmp/root/var/lib/ghostline/bin/zapret2" ] || { echo "FAIL engine files removed ($args)"; fail=1; }
	expect "self-install retired ($args)" "$retired"
done
# Arch (build/linux/aur/ghostline.install): the same retirement on install
# and upgrade; without a self-install, install only prints how to start.
arch() { # arch <function>
	(PATH="$tmp/bin:$PATH" GHOSTLINE_TEST_ROOT="$tmp/root"; . "$here/../../aur/ghostline.install"; "$1") > /dev/null
}
archretired=$'systemctl disable --now ghostline.service\nsystemctl daemon-reload\nsystemctl enable --now ghostline.service'
for fn in post_install post_upgrade; do
	setup; selfunit; arch $fn
	[ -e "$tmp/root/etc/systemd/system/ghostline.service" ] && { echo "FAIL arch self-install left ($fn)"; fail=1; }
	expect "arch self-install retired ($fn)" "$archretired"
done
setup; arch post_install; expect "arch install starts nothing" ""
setup; arch post_upgrade; expect "arch upgrade" $'systemctl daemon-reload\nsystemctl try-restart ghostline.service'

# Someone's own unit in /etc (not Ghostline's self-install) is left alone.
setup; mkdir -p "$tmp/root/etc/systemd/system"; printf '[Service]\nExecStart=/usr/lib/ghostline/ghostlined --daemon\nNice=5\n' > "$tmp/root/etc/systemd/system/ghostline.service"
run postinst.sh configure
[ -e "$tmp/root/etc/systemd/system/ghostline.service" ] || { echo "FAIL admin's override removed"; fail=1; }
expect "admin override kept" "$fresh"
exit $fail
