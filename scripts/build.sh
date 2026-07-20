#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p dist
VERSION="${VERSION:-0.1.5}"
LDFLAGS="-s -w -X main.version=${VERSION}"

build() {
  local goos=$1 goarch=$2
  local out="dist/clipremote_${VERSION}_${goos}_${goarch}"
  if [[ "$goos" == "windows" ]]; then out="${out}.exe"; fi
  echo "building $out"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -buildvcs=false -ldflags "$LDFLAGS" -o "$out" ./cmd/clipremote
}

build darwin arm64
build darwin amd64
build linux amd64
build linux arm64

# local name
cp "dist/clipremote_${VERSION}_$(go env GOOS)_$(go env GOARCH)" dist/clipremote 2>/dev/null || true
echo "done → dist/"
ls -la dist/
