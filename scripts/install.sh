#!/usr/bin/env bash
# Install clipremote for the current machine (local Mac or remote Linux).
set -euo pipefail

REPO="${CLIPREMOTE_REPO:-https://github.com/clipremote/clipremote}"
VERSION="${CLIPREMOTE_VERSION:-0.1.0}"
PREFIX="${PREFIX:-$HOME/.local}"
BIN_DIR="${PREFIX}/bin"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH"; exit 1 ;;
esac

mkdir -p "$BIN_DIR"

if [[ -n "${CLIPREMOTE_BIN:-}" && -f "${CLIPREMOTE_BIN}" ]]; then
  cp "$CLIPREMOTE_BIN" "$BIN_DIR/clipremote"
elif command -v go >/dev/null 2>&1 && [[ -f "$(dirname "$0")/../go.mod" ]]; then
  echo "building from source..."
  ROOT="$(cd "$(dirname "$0")/.." && pwd)"
  go build -buildvcs=false -ldflags "-s -w -X main.version=${VERSION}" -o "$BIN_DIR/clipremote" "$ROOT/cmd/clipremote"
else
  URL="${REPO}/releases/download/v${VERSION}/clipremote_${VERSION}_${OS}_${ARCH}"
  echo "downloading $URL"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "$BIN_DIR/clipremote"
  else
    wget -qO "$BIN_DIR/clipremote" "$URL"
  fi
fi

chmod +x "$BIN_DIR/clipremote"
echo "installed: $BIN_DIR/clipremote"
echo "ensure on PATH: export PATH=\"$BIN_DIR:\$PATH\""

if [[ "${1:-}" == "--remote" ]]; then
  "$BIN_DIR/clipremote" setup --remote "$@"
else
  "$BIN_DIR/clipremote" setup
fi
