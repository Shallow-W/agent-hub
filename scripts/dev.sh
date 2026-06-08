#!/bin/bash
# 启动开发环境：数据库 + 后端 + 前端
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

echo "启动 PostgreSQL..."
docker compose up -d postgres

echo "等待数据库就绪..."
until docker compose exec -T postgres pg_isready -U agenthub -q; do
  sleep 1
done
echo "数据库就绪"

echo "运行迁移..."
for f in src/backend/migrations/*.sql; do
  PGPASSWORD=agenthub psql -h localhost -U agenthub -d agenthub -f "$f"
done

echo "启动后端服务..."
(
  cd src/backend
  "$GO_BIN" run ./cmd/server
) &
BACKEND_PID=$!

echo "启动前端开发服务器..."
(
  cd src/frontend
  npm run dev
) &
FRONTEND_PID=$!

echo "开发环境已启动 (后端 PID: $BACKEND_PID, 前端 PID: $FRONTEND_PID)"
echo "按 Ctrl+C 停止所有服务"

cleanup() {
  echo "停止服务..."
  kill "$BACKEND_PID" "$FRONTEND_PID" 2>/dev/null || true
  docker compose down
}
trap cleanup EXIT INT TERM

wait
