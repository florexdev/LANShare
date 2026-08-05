@echo off
echo Building LANShare Single Portable Executables...
echo.

set CGO_ENABLED=0

echo Building Windows Executable (lanshare-windows.exe)...
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o dist/lanshare-windows.exe .

echo Building Linux Executable (lanshare-linux)...
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o dist/lanshare-linux .

echo Building macOS Executable (lanshare-macos)...
set GOOS=darwin
set GOARCH=amd64
go build -ldflags="-s -w" -o dist/lanshare-macos .

echo.
echo Build Complete! Binary files saved in dist/ directory.
pause
