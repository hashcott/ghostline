#!/bin/sh
# Prints the PKGBUILD for a release: render.sh <version> <tarball> [--local]
# --local points source= at the tarball's file name (makepkg next to it),
# otherwise at the GitHub release.
set -eu
[ $# -ge 2 ] || { echo "usage: render.sh <version> <tarball> [--local]" >&2; exit 2; }
ver=$1
tarball=$2
here=$(cd "$(dirname "$0")" && pwd)
sha=$(sha256sum "$tarball" | cut -d' ' -f1)
name=$(basename "$tarball")
if [ "${3:-}" = --local ]; then
	source=$name
else
	source="https://github.com/hashcott/ghostline/releases/download/v$ver/$name"
fi
# pkgver may not contain "-": 0.6.0-rc1 → 0.6.0_rc1.
pkgver=$(printf '%s' "$ver" | tr - _)
sed -e "s|@PKGVER@|$pkgver|" -e "s|@VERSION@|$ver|" -e "s|@SOURCE@|$source|" -e "s|@SHA256@|$sha|" "$here/PKGBUILD"
