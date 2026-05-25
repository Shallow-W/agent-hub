# Git 开发规范

## 分支命名

| 分支类型 | 命名格式 | 示例 |
|----------|----------|------|
| 功能开发 | `feature/<简短描述>` | `feature/im-chat-ui` |
| 缺陷修复 | `fix/<简短描述>` | `fix/websocket-reconnect` |
| 重构 | `refactor/<简短描述>` | `refactor/adapter-interface` |
| 文档 | `docs/<简短描述>` | `docs/api-design` |

- 描述使用小写英文，单词间用 `-` 连接
- 禁止在分支名中使用中文、特殊字符、大写字母

## Commit Message 格式

```
<type>(<scope>): <description>

[可选的详细说明]
```

### Type 枚举

| Type | 用途 |
|------|------|
| `feat` | 新功能 |
| `fix` | 缺陷修复 |
| `refactor` | 重构（不改变功能） |
| `docs` | 文档变更 |
| `test` | 测试相关 |
| `chore` | 构建、工具、依赖变更 |
| `style` | 代码格式调整（不影响逻辑） |

### Scope 参考

- `agent` — Agent接入层/适配器
- `api` — 后端API
- `auth` — 用户鉴权
- `chat` — 聊天核心功能
- `claude` — CLAUDE.md/开发规范
- `daemon` — 本地守护进程
- `db` — 数据库/数据模型
- `orchestrator` — Orchestrator调度
- `preview` — 产物预览
- `harness` — 开发规范/工程约定（已迁移至 `conventions`）
- `conventions` — 开发规范/工程约定
- `doc` — 文档变更
- `ui` — 前端UI组件

### 规则

- description 使用中文，简洁描述"做了什么"
- 不超过50个字符
- 不以句号结尾
- 一个 commit 只做一件事

### 示例

```
feat(chat): 实现WebSocket流式消息推送
fix(daemon): 修复守护进程断连后未自动重连的问题
refactor(agent): 统一适配器接口，抽取公共解析逻辑
docs(api): 补充REST API接口文档
```

## 工作流

```
main ──────────────────────────────────────────
  │
  ├── feature/im-chat-ui ──→ PR ──→ merge
  │
  └── fix/websocket-reconnect ──→ PR ──→ merge
```

1. 从 `main` 拉取最新代码创建 feature 分支
2. 在 feature 分支上开发，频繁小粒度提交
3. 开发完成后创建 PR（或直接请求合并）
4. 合并后删除 feature 分支

## 禁止事项

- 禁止直推 `main` 分支
- 禁止提交敏感信息（API Key、密码、.env 文件）
- 禁止提交大型二进制文件
- 禁止 `--no-verify` 跳过 hooks
- 禁止 `git push --force` 到共享分支

## .gitignore 必备项

```
# 依赖
node_modules/
vendor/

# 构建产物
dist/
build/
*.exe

# 环境变量
.env
.env.local

# IDE
.idea/
.vscode/
*.swp

# 系统文件
.DS_Store
Thumbs.db

# 日志
*.log
```
