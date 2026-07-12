#!/bin/sh
set -e

# Usage:
#   curl -sSfL https://raw.githubusercontent.com/KIRKR101/hardcover-cli/main/install.sh | sh
#
# Or with a specific version:
#   curl -sSfL https://raw.githubusercontent.com/KIRKR101/hardcover-cli/main/install.sh | sh -s -- v0.1.0

REPO="KIRKR101/hardcover-cli"
BINARY="hardcover"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  msys*|mingw*|cygwin*) OS="windows" ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest version if not specified
if [ -z "$1" ]; then
  VERSION=$(curl -sSfL "https://api.github.com/repos/$REPO/releases/latest" | { command -v jq >/dev/null 2>&1 && jq -r '.tag_name' || grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'; })
else
  VERSION="$1"
fi

if [ -z "$VERSION" ]; then
  echo "Failed to determine version"
  exit 1
fi

# Determine archive format
if [ "$OS" = "windows" ]; then
  EXT="zip"
else
  EXT="tar.gz"
fi

ARCHIVE="${REPO##*/}_${VERSION#v}_${OS}_${ARCH}.${EXT}"
URL="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE"

echo "Installing $BINARY $VERSION for $OS/$ARCH..."

# Download
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
cd "$TMP_DIR"

if ! curl -sSfL "$URL" -o "$ARCHIVE"; then
  echo "Failed to download $URL"
  exit 1
fi

# Verify checksum
CHECKSUM_URL="https://github.com/$REPO/releases/download/$VERSION/checksums.txt"
if curl -sSfL "$CHECKSUM_URL" -o checksums.txt 2>/dev/null; then
  if command -v sha256sum >/dev/null 2>&1; then
    EXPECTED=$(grep "  *$ARCHIVE\$" checksums.txt | awk '{print $1}')
    ACTUAL=$(sha256sum "$ARCHIVE" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    EXPECTED=$(grep "  *$ARCHIVE\$" checksums.txt | awk '{print $1}')
    ACTUAL=$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')
  else
    EXPECTED=""
  fi
  if [ -n "$EXPECTED" ] && [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "Checksum mismatch!"
    echo "  Expected: $EXPECTED"
    echo "  Actual:   $ACTUAL"
    exit 1
  fi
fi

# Extract
if [ "$EXT" = "zip" ]; then
  unzip -q "$ARCHIVE"
else
  tar -xzf "$ARCHIVE"
fi

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "$BINARY" "$INSTALL_DIR/"
else
  sudo mv "$BINARY" "$INSTALL_DIR/"
fi

chmod +x "$INSTALL_DIR/$BINARY"

echo "Installed $BINARY to $INSTALL_DIR/$BINARY"
echo "Run '$BINARY --help' to get started"
