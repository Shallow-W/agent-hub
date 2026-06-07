#!/bin/bash
# 运行后端 Go 测试
set -e

cd src/backend
exec go test ./internal/... -count=1 -timeout 60s "$@"
