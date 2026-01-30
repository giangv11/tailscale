#!/bin/sh
# Test script to verify install-system-daemon command works on NetBSD

echo "Testing tailscaled install-system-daemon command..."
echo ""

# Check if binary exists
if [ ! -f "./tailscaled" ]; then
    echo "ERROR: tailscaled binary not found in current directory"
    exit 1
fi

# Check if it's executable
if [ ! -x "./tailscaled" ]; then
    echo "ERROR: tailscaled is not executable"
    exit 1
fi

# Try to run the command (dry run - just check if it's recognized)
echo "Checking if 'install-system-daemon' subcommand is recognized..."
./tailscaled install-system-daemon --help 2>&1 || echo "Command exists but may need sudo"

echo ""
echo "If you see an error about 'not available on netbsd', the binary needs to be rebuilt."
echo "If you see permission errors, run with sudo."
echo ""
echo "To rebuild on NetBSD:"
echo "  cd /path/to/tailscale"
echo "  go build -o tailscaled ./cmd/tailscaled"
echo ""
echo "Then run:"
echo "  sudo ./tailscaled install-system-daemon"
