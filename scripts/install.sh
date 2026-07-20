#!/usr/bin/env bash
# Install clipremote for the current machine (local Mac or remote Linux).
set -euo pipefail

REPO="${CLIPREMOTE_REPO:-https://github.com/vulcanhelix/clipremote}"
VERSION="${CLIPREMOTE_VERSION:-0.1.5}"
PREFIX="${PREFIX:-$HOME/.local}"
BIN_DIR="${PREFIX}/bin"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Go may live here even if not on PATH
export PATH="/usr/local/go/bin:${HOME}/go/bin:${PATH}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH"; exit 1 ;;
esac

mkdir -p "$BIN_DIR"

LOCAL_DIST="${ROOT}/dist/clipremote_${VERSION}_${OS}_${ARCH}"
LOCAL_GENERIC="${ROOT}/dist/clipremote"

if [[ -n "${CLIPREMOTE_BIN:-}" && -f "${CLIPREMOTE_BIN}" ]]; then
  echo "using CLIPREMOTE_BIN=${CLIPREMOTE_BIN}"
  cp "$CLIPREMOTE_BIN" "$BIN_DIR/clipremote"
elif [[ -f "$LOCAL_DIST" ]]; then
  echo "installing from $LOCAL_DIST"
  cp "$LOCAL_DIST" "$BIN_DIR/clipremote"
elif [[ -f "$LOCAL_GENERIC" ]]; then
  echo "installing from $LOCAL_GENERIC"
  cp "$LOCAL_GENERIC" "$BIN_DIR/clipremote"
elif command -v go >/dev/null 2>&1 && [[ -f "$ROOT/go.mod" ]]; then
  echo "building from source..."
  go build -buildvcs=false -ldflags "-s -w -X main.version=${VERSION}" \
    -o "$BIN_DIR/clipremote" "$ROOT/cmd/clipremote"
else
  URL="${REPO}/releases/download/v${VERSION}/clipremote_${VERSION}_${OS}_${ARCH}"
  echo "downloading $URL"
  if ! curl -fsSL "$URL" -o "$BIN_DIR/clipremote"; then
    echo "error: no GitHub release yet, and no local binary/source build available." >&2
    echo "  fix: from the repo run:  ./scripts/build.sh && ./scripts/install.sh" >&2
    echo "  or:  CLIPREMOTE_BIN=./dist/clipremote ./scripts/install.sh" >&2
    exit 1
  fi
fi

chmod +x "$BIN_DIR/clipremote"
echo "installed: $BIN_DIR/clipremote"
echo "ensure on PATH: export PATH=\"$BIN_DIR:\$PATH\""

if [[ "${1:-}" == "--remote" ]] || [[ "$*" == *"--remote"* ]]; then
  "$BIN_DIR/clipremote" setup --remote
else
  "$BIN_DIR/clipremote" setup
fi
