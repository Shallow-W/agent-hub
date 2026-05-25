#!/bin/bash
# 构建后端二进制 + 前端产物
set -e

echo "构建后端..."
cd src/backend
CGO_ENABLED=0 go build -o ../../bin/server cmd/server/main.go
cd ../..

echo "构建前端..."
cd src/frontend
npm run build
cd ../..

echo "构建完成: bin/server + src/frontend/dist/"
