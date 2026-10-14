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
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
cp -a packaging/deb/. "$STAGE/"
# Binary: prefer an existing build/hinatracer-linux-amd64, else build one.
BIN="$ROOT/build/hinatracer-linux-amd64"
if [[ ! -f "$BIN" ]]; then
  echo "building linux amd64 binary…"
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
} > "$STAGE/usr/share/doc/hinatracer-server/copyright"
DEB="$OUT/HinaTracer-v${VERSION}-linux-amd64-server.deb"
# Prefer dpkg-deb; fall back to a plain ar archive if unavailable.
if command -v dpkg-deb >/dev/null 2>&1; then
  dpkg-deb --root-owner-group --build "$STAGE" "$DEB"
else
  echo "dpkg-deb not found; packing with ar (lintian will not run)" >&2
  (cd "$STAGE" && tar czf "$STAGE/../data.tar.gz" --exclude=DEBIAN .)
  (cd "$STAGE/DEBIAN" && tar czf "$STAGE/../control.tar.gz" .)
  echo "2.0" > "$STAGE/../debian-binary"
  ar r "$DEB" "$STAGE/../debian-binary" "$STAGE/../control.tar.gz" "$STAGE/../data.tar.gz"
fi
echo "wrote $DEB"
ls -la "$DEB"
