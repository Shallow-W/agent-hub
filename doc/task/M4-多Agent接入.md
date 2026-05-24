# M4 多 Agent 接入

## 目标

实现本地守护进程，扫描已安装的 Agent CLI，通过统一适配器层与它们通信。

## 子任务

### M4-1 守护进程连接后端

- 守护进程启动时通过 WebSocket 主动连接 Go 后端
- 注册本机信息（机器名、可用 Agent 列表）
- 心跳保活，断连自动重连
- 后端维护 守护进程连接池（支持多台机器）

### M4-2 Agent 发现/扫描

- 扫描 PATH 中已知 CLI 命令：`claude`, `codex`, `opencode` 等
- 对每个发现的 CLI 执行 `--version` 验证可用性
- 上报发现结果给后端
- 定期重新扫描（或手动触发）

### M4-3 适配器统一接口

定义 Adapter 接口：

```go
type Adapter interface {
    Name() string
    Start(ctx context.Context, prompt string, systemPrompt string) error
    Stream() <-chan StreamChunk  // 流式输出通道
    Stop() error
    IsRunning() bool
}

type StreamChunk struct {
    Type    string // "text" | "artifact" | "error" | "done"
    Content string
    Artifact *Artifact // 非空时为结构化产物
}
```

### M4-4 Claude Code 适配器

- 通过 `claude` CLI 启动进程
- stdin 传入用户消息 + system prompt
- stdout 流式读取输出
- 解析 Markdown 输出，提取代码块为结构化产物
- 错误处理（CLI 未安装、进程崩溃、超时）

### M4-5 Codex/OpenCode 适配器

- 与 M4-4 类似，针对对应 CLI 的输入输出格式做适配
- 至少实现一个非 Claude Code 的适配器

### M4-6 Agent 配置 CRUD

- `GET /api/agents` — 获取可用 Agent 列表（来自守护进程上报 + 用户自建）
- `POST /api/agents` — 创建自建 Agent
- `PUT /api/agents/:id` — 修改自建 Agent
- `DELETE /api/agents/:id` — 删除自建 Agent
- 前端 Agent 列表 UI

## 验收标准

- [ ] 守护进程可连接后端并保持 WebSocket 长连接
- [ ] 守护进程能扫描到本地安装的 Claude Code CLI
- [ ] Claude Code 适配器可启动 CLI 进程并获取流式输出
- [ ] 至少有两个不同类型的适配器实现
- [ ] Agent 列表在 WebUI 中正确展示

## 依赖

- M0-3（守护进程骨架）
- M2-1（后端 WebSocket）
- M3-3（前端侧边栏展示 Agent）
