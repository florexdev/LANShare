#!/bin/bash
echo "Building LANShare Single Portable Executables..."
echo ""

export CGO_ENABLED=0

mkdir -p dist

echo "Building Windows Executable (lanshare-windows.exe)..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/lanshare-windows.exe .

echo "Building Linux Executable (lanshare-linux)..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/lanshare-linux .

echo "Building macOS Executable (lanshare-macos)..."
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/lanshare-macos .

echo ""
echo "Build Complete! Binary files saved in dist/ directory."
