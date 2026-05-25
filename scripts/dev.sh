#!/bin/bash
# 启动开发环境：数据库 + 后端 + 前端
set -e

echo "启动 PostgreSQL..."
docker compose up -d postgres

# 等待数据库就绪
echo "等待数据库就绪..."
until docker compose exec -T postgres pg_isready -U agenthub -q; do
  sleep 1
done
echo "数据库就绪"

# 运行数据库迁移
echo "运行迁移..."
for f in src/backend/migrations/*.sql; do
  PGPASSWORD=agenthub psql -h localhost -U agenthub -d agenthub -f "$f"
done

# 启动后端（后台）
echo "启动后端服务..."
cd src/backend && go run cmd/server/main.go &
BACKEND_PID=$!
cd ../..

# 启动前端
echo "启动前端开发服务器..."
cd src/frontend && npm run dev &
FRONTEND_PID=$!
cd ../..

echo "开发环境已启动 (后端 PID: $BACKEND_PID, 前端 PID: $FRONTEND_PID)"
echo "按 Ctrl+C 停止所有服务"

cleanup() {
  echo "停止服务..."
  kill $BACKEND_PID $FRONTEND_PID 2>/dev/null
  docker compose down
}
trap cleanup EXIT INT TERM

wait
