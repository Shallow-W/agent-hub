#!/bin/bash
# 运行后端 Go 测试
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

cd src/backend
exec "$GO_BIN" test ./internal/... -count=1 -timeout 60s "$@"
