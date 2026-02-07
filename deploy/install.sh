#!/usr/bin/env bash
set -euo pipefail

# PVE App Store — one-liner installer
# Usage: curl -fsSL <url>/install.sh | bash

REPO="inertz/pve-appstore"
INSTALL_DIR="/opt/pve-appstore"
BINARY="pve-appstore"

echo "=== PVE App Store Installer ==="
echo

# Must be root
if [ "$(id -u)" -ne 0 ]; then
    echo "Error: this installer must be run as root."
    exit 1
fi

# Must be Proxmox
if ! command -v pveversion &>/dev/null; then
    echo "Error: this doesn't appear to be a Proxmox VE host (pveversion not found)."
    exit 1
fi

echo "Proxmox VE detected: $(pveversion)"
echo

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH_SUFFIX="linux-amd64" ;;
    aarch64) ARCH_SUFFIX="linux-arm64" ;;
    *)
        echo "Error: unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Download latest release
echo "Downloading ${BINARY}-${ARCH_SUFFIX}..."
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${BINARY}-${ARCH_SUFFIX}"

mkdir -p "$INSTALL_DIR"
if command -v curl &>/dev/null; then
    curl -fsSL "$DOWNLOAD_URL" -o "${INSTALL_DIR}/${BINARY}"
elif command -v wget &>/dev/null; then
    wget -qO "${INSTALL_DIR}/${BINARY}" "$DOWNLOAD_URL"
else
    echo "Error: neither curl nor wget found."
    exit 1
fi

chmod 0755 "${INSTALL_DIR}/${BINARY}"
echo "Installed to ${INSTALL_DIR}/${BINARY}"
echo

# Run the interactive TUI installer
exec "${INSTALL_DIR}/${BINARY}" install
