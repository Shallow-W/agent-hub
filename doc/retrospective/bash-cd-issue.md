# 复盘：Bash 工具 cd 重复错误

## 问题
连续 10+ 次调用 `go build -o bin/server ./cmd/server/` 失败，错误信息为 `go: cannot find main module`。

## 根因
**在 `description` 参数中写路径说明，但 `command` 参数中没有 `cd`。**

- `description` 只是人类可读标签，不影响命令执行
- `command` 才是实际执行的 shell 命令
- Bash 工具的工作目录默认为项目根 `/Users/shallow/Desktop/repo/agent-hub`
- Go module 在 `src/backend/go.mod`，必须先 cd 到该目录

## 正确写法

```bash
# ✅ 正确 — command 包含 cd
cd /Users/shallow/Desktop/repo/agent-hub/src/backend && go build -o bin/server ./cmd/server/

# ❌ 错误 — command 没有 cd，description 有 cd 说明（无用）
# command: go build -o bin/server ./cmd/server/
# description: "Build backend from src/backend directory"
```

## 预防措施
1. 本项目的 Go 代码在 `src/backend/`，npm 前端在 `src/frontend/`
2. 每次执行 Go 命令时，command 必须以 `cd /Users/shallow/Desktop/repo/agent-hub/src/backend &&` 开头
3. 每次执行前端 npm/node 命令时，command 必须以 `cd /Users/shallow/Desktop/repo/agent-hub/src/frontend &&` 开头
4. 写 Bash tool call 时，先写完 command 参数，确认包含 cd，再写 description
5. 连续失败 2 次同一条命令时，立即检查 command 内容而非重试

## 相关项目结构
```
/Users/shallow/Desktop/repo/agent-hub/     ← 项目根（默认工作目录）
  src/backend/                              ← Go module 根（go.mod 在此）
  src/frontend/                             ← 前端项目根（package.json 在此）
```
