#!/usr/bin/env bash
# Davit installer — downloads the correct binary for this machine and installs it.
# Usage: curl -fsSL https://raw.githubusercontent.com/getdavit/davit/main/scripts/install.sh | sudo bash
set -euo pipefail

REPO="getdavit/davit"
INSTALL_DIR="/usr/local/bin"
BINARY="davit"

# ── detect OS ──────────────────────────────────────────────────────────────────

OS="$(uname -s)"
if [ "$OS" != "Linux" ]; then
    echo "error: davit only supports Linux (got $OS)" >&2
    exit 1
fi

# ── detect architecture ────────────────────────────────────────────────────────

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)
        echo "error: unsupported architecture: $ARCH" >&2
        exit 1
        ;;
esac

# ── resolve version ────────────────────────────────────────────────────────────

VERSION="${DAVIT_VERSION:-}"
if [ -z "$VERSION" ]; then
    VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
fi

if [ -z "$VERSION" ]; then
    echo "error: could not determine latest release version" >&2
    exit 1
fi

ASSET="davit-linux-${ARCH}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

# ── download and install ───────────────────────────────────────────────────────

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

echo "Downloading davit ${VERSION} (linux/${ARCH})..."
curl -fsSL -o "$TMP" "$URL"
chmod +x "$TMP"

# Verify the binary runs
if ! "$TMP" --version >/dev/null 2>&1; then
    echo "error: downloaded binary failed self-check" >&2
    exit 1
fi

install -m 755 "$TMP" "${INSTALL_DIR}/${BINARY}"

echo "Installed: ${INSTALL_DIR}/${BINARY}"
"${INSTALL_DIR}/${BINARY}" --version
