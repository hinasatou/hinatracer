#!/usr/bin/env bash
# Build the headless server .deb (linux amd64, CGO_ENABLED=0).
# Usage: scripts/build-server-deb.sh [version] [outdir]
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  VERSION="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' mygo.json | head -n1)"
fi
OUT="${2:-$ROOT/build}"
mkdir -p "$OUT"
# Work in a private temp dir: the package root and the ar fallback files
# live side by side in it, never next to other jobs' files.
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
STAGE="$WORK/root"
DOC="$STAGE/usr/share/doc/hinatracer-server"
# Git does not keep empty directories, so create every directory written
# below instead of relying on the checkout.
mkdir -p "$STAGE/DEBIAN" "$STAGE/usr/bin" "$DOC" "$STAGE/etc/hinatracer" "$STAGE/lib/systemd/system"
for f in DEBIAN/conffiles DEBIAN/postinst DEBIAN/prerm DEBIAN/postrm \
  lib/systemd/system/hinatracer.service etc/hinatracer/config.json; do
  if [[ ! -f "packaging/deb/$f" ]]; then
    echo "missing packaging/deb/$f (is it committed?)" >&2
    exit 1
  fi
done
cp -R packaging/deb/. "$STAGE/"
# Checkouts made on Windows lose the executable bit; dpkg needs these modes.
chmod 0755 "$STAGE/DEBIAN" "$STAGE/DEBIAN/postinst" "$STAGE/DEBIAN/prerm" "$STAGE/DEBIAN/postrm"
chmod 0644 "$STAGE/DEBIAN/conffiles" "$STAGE/lib/systemd/system/hinatracer.service" "$STAGE/etc/hinatracer/config.json"
# Strip CR in case the files were checked out with CRLF line endings.
sed -i 's/\r$//' "$STAGE/DEBIAN/conffiles" "$STAGE/DEBIAN/postinst" "$STAGE/DEBIAN/prerm" "$STAGE/DEBIAN/postrm" \
  "$STAGE/lib/systemd/system/hinatracer.service"
# Binary: prefer an existing build/hinatracer-linux-amd64, else build one.
BIN="$ROOT/build/hinatracer-linux-amd64"
if [[ ! -f "$BIN" ]]; then
  echo "building linux amd64 binary…"
  mkdir -p "$(dirname "$BIN")"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$BIN" .
fi
install -D -m 0755 "$BIN" "$STAGE/usr/bin/hinatracer"
SIZE="$(du -sk "$STAGE" | awk '{print $1}')"
cat > "$STAGE/DEBIAN/control" <<CTRL
Package: hinatracer-server
Version: ${VERSION}
Section: net
Priority: optional
Architecture: amd64
Maintainer: HinaTracer <https://github.com/hinasatou/hinatracer>
Depends: adduser, libc6
Installed-Size: ${SIZE}
Homepage: https://github.com/hinasatou/hinatracer
Description: HinaTracer headless WebUI server
 Continuously ping and traceroute hosts and serve the HinaTracer WebUI
 (and MCP endpoint) under systemd. Does not install the desktop GUI.
CTRL
{
  echo "Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/"
  echo "Upstream-Name: HinaTracer"
  echo "Source: https://github.com/hinasatou/hinatracer"
  echo
  echo "Files: *"
  echo "Copyright: 2026 hinasatou (https://github.com/hinasatou/hinatracer)"
  echo "License: MIT"
  sed -n '/^Permission is hereby granted/,$p' LICENSE | sed 's/^$/./; s/^/ /'
} > "$DOC/copyright"
chmod 0644 "$DOC/copyright"
DEB="$OUT/HinaTracer-v${VERSION}-linux-amd64-server.deb"
# Prefer dpkg-deb; fall back to a plain ar archive if unavailable.
if command -v dpkg-deb >/dev/null 2>&1; then
  dpkg-deb --root-owner-group --build "$STAGE" "$DEB"
else
  echo "dpkg-deb not found; packing with ar (lintian will not run)" >&2
  (cd "$STAGE" && tar czf "$WORK/data.tar.gz" --owner=0 --group=0 --exclude=./DEBIAN .)
  (cd "$STAGE/DEBIAN" && tar czf "$WORK/control.tar.gz" --owner=0 --group=0 .)
  echo "2.0" > "$WORK/debian-binary"
  rm -f "$DEB"
  ar rc "$DEB" "$WORK/debian-binary" "$WORK/control.tar.gz" "$WORK/data.tar.gz"
fi
echo "wrote $DEB"
ls -la "$DEB"
