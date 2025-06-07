#!/bin/bash

# Build script for caddy-usage plugin
# This script builds Caddy with the usage plugin using xcaddy

set -e

echo "Building Caddy with usage plugin..."

# Check if xcaddy is installed
if ! command -v xcaddy &> /dev/null; then
    echo "xcaddy not found. Installing xcaddy..."
    go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
fi

# Build Caddy with the usage plugin
echo "Building Caddy with caddy-usage plugin..."
xcaddy build --with github.com/chalabi/caddy-usage

# Make the binary executable
chmod +x caddy

echo "Build complete! Caddy binary with usage plugin is ready."
echo "You can now run: ./caddy run --config example-configs/Caddyfile"
echo "Or: ./caddy run --config example-configs/caddy.json"
echo ""
echo "Metrics will be available at: http://localhost:2019/metrics" 