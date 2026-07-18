#!/usr/bin/env bash
# Build helper for the HWiNFO GUI recreation (Go + miqt/Qt6).
#
# This project uses the miqt Qt6 bindings, which compile via cgo against the
# Qt6 development headers. On this machine the Qt6 *runtime* is installed
# system-wide, but the *devel* package (headers, moc, pkg-config files) was
# unpacked into a local prefix so no root/sudo was required.
#
# For a normal setup just run:  sudo dnf install qt6-qtbase-devel  &&  go build
# and you can ignore QT_LOCAL_PREFIX below.

set -euo pipefail

# Local, sudo-free Qt6 devel prefix (extracted qt6-qtbase-devel RPM).
QT_LOCAL_PREFIX="${QT_LOCAL_PREFIX:-/tmp/claude-1000/-home-zen66ten-Documents-Code-gui/2a72f188-50fc-4f22-9566-6a8a14c4b8d1/scratchpad/qtdev/usr}"

if [ -d "$QT_LOCAL_PREFIX/lib64/pkgconfig" ]; then
  export PKG_CONFIG_PATH="$QT_LOCAL_PREFIX/lib64/pkgconfig:${PKG_CONFIG_PATH:-}"
  export PATH="$QT_LOCAL_PREFIX/lib64/qt6/libexec:$PATH"   # moc/uic/rcc if needed
fi

export CGO_ENABLED=1
exec go "${1:-build}" "${@:2}"
