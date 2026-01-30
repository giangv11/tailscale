#!/bin/sh
# Build and install script for Tailscale on NetBSD
# Run this script on your NetBSD system

set -e

echo "=========================================="
echo "Tailscale NetBSD Build and Install Script"
echo "=========================================="
echo ""

# Check if we're on NetBSD
if [ "$(uname -s)" != "NetBSD" ]; then
    echo "ERROR: This script must be run on NetBSD"
    exit 1
fi

# Check if Go is installed
if ! command -v go >/dev/null 2>&1; then
    echo "ERROR: Go is not installed. Please install Go first."
    exit 1
fi

echo "Step 1: Building tailscaled..."
echo "-----------------------------------"
go build -v -o tailscaled ./cmd/tailscaled

if [ ! -f "./tailscaled" ]; then
    echo "ERROR: Build failed - tailscaled binary not found"
    exit 1
fi

echo ""
echo "Build successful!"
echo ""

# Check if install command is available
echo "Step 2: Verifying install-system-daemon command..."
echo "-----------------------------------"
if ./tailscaled install-system-daemon 2>&1 | grep -q "not available"; then
    echo "ERROR: install-system-daemon command is not available."
    echo "This means install_netbsd.go was not included in the build."
    echo "Please check that you're building on NetBSD and the file exists."
    exit 1
fi

echo "Command is available!"
echo ""

# Ask for confirmation before installing
echo "Step 3: Ready to install"
echo "-----------------------------------"
echo "This will:"
echo "  - Copy tailscaled to /usr/sbin/tailscaled"
echo "  - Create /var/lib/tailscale and /var/run/tailscale directories"
echo "  - Install rc.d script to /etc/rc.d/tailscaled"
echo "  - Add tailscaled=YES to /etc/rc.conf"
echo "  - Start the tailscaled service"
echo ""
read -p "Do you want to proceed with installation? (yes/no): " confirm

if [ "$confirm" != "yes" ]; then
    echo "Installation cancelled."
    exit 0
fi

echo ""
echo "Step 4: Installing..."
echo "-----------------------------------"
sudo ./tailscaled install-system-daemon

echo ""
echo "=========================================="
echo "Installation complete!"
echo "=========================================="
echo ""
echo "To verify installation:"
echo "  /etc/rc.d/tailscaled status"
echo "  ifconfig tun0"
echo "  grep tailscaled /etc/rc.conf"
echo ""
