#!/bin/sh
# Script to verify Tailscale installation on NetBSD

echo "=========================================="
echo "Verifying Tailscale Installation"
echo "=========================================="
echo ""

echo "1. Checking if binary is installed..."
if [ -f "/usr/sbin/tailscaled" ]; then
    echo "   ✓ tailscaled is installed at /usr/sbin/tailscaled"
else
    echo "   ✗ tailscaled not found at /usr/sbin/tailscaled"
fi

echo ""
echo "2. Checking if rc.d script exists..."
if [ -f "/etc/rc.d/tailscaled" ]; then
    echo "   ✓ rc.d script exists at /etc/rc.d/tailscaled"
    if [ -x "/etc/rc.d/tailscaled" ]; then
        echo "   ✓ rc.d script is executable"
    else
        echo "   ✗ rc.d script is not executable"
    fi
else
    echo "   ✗ rc.d script not found"
fi

echo ""
echo "3. Checking if service is enabled in rc.conf..."
if grep -q "^tailscaled=YES" /etc/rc.conf 2>/dev/null; then
    echo "   ✓ tailscaled=YES found in /etc/rc.conf"
elif grep -q "tailscaled=" /etc/rc.conf 2>/dev/null; then
    echo "   ⚠ tailscaled entry found but may not be YES:"
    grep "tailscaled=" /etc/rc.conf
else
    echo "   ✗ tailscaled not found in /etc/rc.conf"
fi

echo ""
echo "4. Checking if directories exist..."
if [ -d "/var/lib/tailscale" ]; then
    echo "   ✓ /var/lib/tailscale exists"
else
    echo "   ✗ /var/lib/tailscale does not exist"
fi

if [ -d "/var/run/tailscale" ]; then
    echo "   ✓ /var/run/tailscale exists"
else
    echo "   ✗ /var/run/tailscale does not exist"
fi

echo ""
echo "5. Checking service status..."
if /etc/rc.d/tailscaled status >/dev/null 2>&1; then
    echo "   ✓ Service is running"
    /etc/rc.d/tailscaled status
else
    echo "   ⚠ Service status check failed (may not be running)"
    echo "   Try: /etc/rc.d/tailscaled start"
fi

echo ""
echo "6. Checking tun0 interface..."
if ifconfig tun0 >/dev/null 2>&1; then
    echo "   ✓ tun0 interface exists"
    ifconfig tun0 | head -5
else
    echo "   ⚠ tun0 interface not found (may appear after service starts)"
fi

echo ""
echo "=========================================="
echo "Verification complete!"
echo "=========================================="
echo ""
echo "If everything looks good, the installation was successful!"
echo "The daemon should start automatically on system boot."
