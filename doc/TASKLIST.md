# AgentHub 任务列表

> 简略索引。每个任务的详细要求、子任务和验收标准见 `doc/task/` 下对应文件。

## 状态说明

`[ ]` 待开始 | `[~]` 进行中 | `[x]` 已完成 | `[-]` 取消

---

## P0

| # | 任务 | 详情 | 依赖 | 状态 |
|---|------|------|------|------|
| M0 | 项目基础设施（脚手架 + 数据库 + API规范） | [doc/task/M0-基础设施.md](task/M0-基础设施.md) | 无 | [ ] |
| M1 | 用户鉴权（注册/登录/JWT） | [doc/task/M1-用户鉴权.md](task/M1-用户鉴权.md) | M0 | [ ] |
| M2 | WebSocket 通信基础设施 | [doc/task/M2-WebSocket通信.md](task/M2-WebSocket通信.md) | M0, M1 | [ ] |
| M3 | IM 聊天核心（对话列表 + 聊天窗口 + 流式消息） | [doc/task/M3-IM聊天核心.md](task/M3-IM聊天核心.md) | M1, M2 | [ ] |
| M4 | 多 Agent 接入（守护进程 + 适配器 + CLI通信） | [doc/task/M4-多Agent接入.md](task/M4-多Agent接入.md) | M0, M2 | [ ] |
| M5 | Orchestrator（意图拆解 + 并行调度 + 聚合） | [doc/task/M5-Orchestrator.md](task/M5-Orchestrator.md) | M4 | [ ] |
| M6 | 单聊端到端跑通 | [doc/task/M6-单聊跑通.md](task/M6-单聊跑通.md) | M3, M4 | [ ] |
| M7 | 群聊端到端跑通 | [doc/task/M7-群聊跑通.md](task/M7-群聊跑通.md) | M5, M6 | [ ] |
| M8 | 自建 Agent（选CLI + 编写Prompt） | [doc/task/M8-自建Agent.md](task/M8-自建Agent.md) | M4, M6 | [ ] |

## P1

| # | 任务 | 详情 | 依赖 | 状态 |
|---|------|------|------|------|
| M9 | 产物预览（结构化卡片：代码/网页/文件） | [doc/task/M9-产物预览.md](task/M9-产物预览.md) | M6 | [ ] |
| M10 | Pin 消息上下文（Pin注入Agent请求） | [doc/task/M10-Pin上下文.md](task/M10-Pin上下文.md) | M3, M4 | [ ] |

---

## 关键路径

```
M0 → M1 → M2 → M3 ──→ M6(单聊跑通) → M7(群聊跑通) → M8(自建Agent)
           ↓     ↘                                    ↘
           M4 ────→ M5 ──────────────────────────→ M7       M9(产物预览)
                                                           M10(Pin上下文)
```

**最短跑通**：M0 → M1 → M2 → M3 → M4 → M6
