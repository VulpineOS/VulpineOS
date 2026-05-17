#!/bin/bash
set -e

echo "Installing VulpineOS..."

# Check for required tools
command -v go >/dev/null 2>&1 || { echo "Error: Go is required but not installed. Install from https://go.dev/dl/" >&2; exit 1; }

# Clone if not already in a vulpineos directory
if [ ! -f "go.mod" ]; then
    echo "Cloning VulpineOS..."
    git clone https://github.com/VulpineOS/VulpineOS.git
    cd VulpineOS
fi

# Install nanoclaw (replaces npm install for OpenClaw)
if command -v npm >/dev/null 2>&1; then
    echo "Installing nanoclaw..."
    npm install -g nanoclaw 2>/dev/null || true
fi

# Build
echo "Building vulpineos..."
go build -o vulpineos ./cmd/vulpineos

echo ""
echo "Installation complete! Run ./vulpineos to start."
echo ""