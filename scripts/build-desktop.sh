#!/bin/bash
# Build script for Build Agent Desktop Application (Linux/macOS)

set -e

echo "========================================"
echo "Building Build Agent Desktop Application"
echo "========================================"
echo ""

# Check if Wails CLI is installed
if ! command -v wails &> /dev/null; then
    echo "[ERROR] Wails CLI not found!"
    echo "Please install it first: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "[ERROR] Go not found!"
    echo "Please install Go 1.24 or later from https://go.dev/dl/"
    exit 1
fi

echo "[1/3] Checking dependencies..."
go mod download

echo ""
echo "[2/3] Building desktop application..."
wails build

echo ""
echo "[3/3] Build completed successfully!"
echo ""
echo "Output: build/bin/build-agent"
echo ""
echo "You can now run the application:"
echo "  ./build/bin/build-agent"
echo ""
