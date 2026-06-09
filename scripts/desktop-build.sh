#!/bin/bash
# Build backend, frontend, and Electron desktop directory package.
set -e

GO_BIN="${GO_BIN:-go}"
if ! command -v "$GO_BIN" >/dev/null 2>&1; then
  if command -v go.exe >/dev/null 2>&1; then
    GO_BIN="go.exe"
  else
    echo "go/go.exe not found in PATH" >&2
    exit 127
  fi
fi

echo "Building backend binary..."
cd src/backend
GOOS_VALUE="$("$GO_BIN" env GOOS)"
if [[ "$GOOS_VALUE" == "windows" ]]; then
  SERVER_OUT="../../bin/server.exe"
else
  SERVER_OUT="../../bin/server"
fi
CGO_ENABLED=0 "$GO_BIN" build -o "$SERVER_OUT" ./cmd/server
cd ../..

echo "Building Electron desktop package..."
cd src/frontend
npm run build:electron
