# Scripts

项目所有构建、测试、启动操作统一通过本目录下的脚本执行。

## build.sh — 构建

```bash
bash scripts/build.sh
```

- 编译后端 Go 二进制 → `bin/server`
- 构建前端 Vite 产物 → `src/frontend/dist/`

## test.sh — 测试

```bash
bash scripts/test.sh              # 运行全部测试
bash scripts/test.sh -run TestFoo # 运行指定测试
bash scripts/test.sh -v           # 详细输出
```

- 运行 `src/backend/internal/...` 下所有 Go 测试
- 额外参数会透传给 `go test`

## dev.sh — 开发环境

```bash
bash scripts/dev.sh
```

- 启动 PostgreSQL（docker compose）
- 等待数据库就绪并运行迁移
- 启动后端 `go run` + 前端 `npm run dev`
- Ctrl+C 停止所有服务

## 注意事项

- 所有脚本从项目根目录执行，内部自行 cd 到子目录
- 不要手动 cd 到 `src/backend` 再运行 go build/test，工作目录不会在命令间保持
