#!/bin/bash
# 构建后端二进制 + 前端产物
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

echo "构建后端..."
cd src/backend
CGO_ENABLED=0 "$GO_BIN" build -o ../../bin/server ./cmd/server
cd ../..

echo "构建前端..."
cd src/frontend
npm run build
cd ../..

echo "构建完成: bin/server + src/frontend/dist/"
