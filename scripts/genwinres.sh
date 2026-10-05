#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
VERSION="$(grep -E '^\s*appVersion\s*=' ui_view.go | head -1 | sed -E 's/.*"([^"]+)".*/\1/')"
if ! command -v go-winres >/dev/null 2>&1; then
  go install github.com/tc-hib/go-winres@latest
fi
export PATH="$(go env GOPATH)/bin:${PATH}"
go-winres simply \
  --icon resources/icon.png \
  --arch amd64 \
  --out rsrc \
  --manifest gui \
  --product-name HinaTracer \
  --file-description HinaTracer \
  --product-version "$VERSION" \
  --file-version "$VERSION" \
  --original-filename hinatracer.exe
