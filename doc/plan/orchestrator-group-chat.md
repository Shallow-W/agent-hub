# Orchestrator 群聊多 Agent 协作 — 详细计划

> 日期: 2026-06-04 | 状态: 规划中 | 优先级: P0

---

## 一、目标

在群聊中实现 Orchestrator（协调器）Agent，能够：
1. 接收用户消息，结合群聊上下文理解意图
2. 拆解任务，选择合适的上下文片段分派给工作 Agent
3. 通过 MCP 调用派活，实时同步状态到任务面板
4. 等待所有 Agent 完成，汇总结果并返回

---

## 二、现有基础

| 模块 | 状态 | 说明 |
|------|------|------|
| OrchestratorService | 部分实现 | 仅有 `RouteMention`（@mention 路由），无意图拆解 |
| DaemonTask | 已实现 | 内存队列，daemon 认领并执行 CLI 任务 |
| Agent CRUD | 已实现 | 系统发现 + 用户自建 Agent |
| MCP 工具 (M11) | 已实现 | daemon 暴露 agenthub-platform MCP server |
| WS Hub / DaemonHub | 已实现 | 实时推送 + daemon 长连接 |
| 前端任务面板 | 基础 UI | 列表展示，缺少实时状态更新 |

---

## 三、核心设计：统一数据结构

### 3.1 OrchestratorTask — 编排任务

```go
// 编排任务：一次用户请求对应一个 OrchestratorTask
type OrchestratorTask struct {
    ID             string            `json:"id"`
    ConversationID string            `json:"conversation_id"`
    UserID         string            `json:"user_id"`          // 发起用户
    TriggerMsgID   string            `json:"trigger_msg_id"`   // 触发消息 ID
    Status         OrchTaskStatus    `json:"status"`           // pending/running/aggregating/done/failed
    Analysis       string            `json:"analysis"`         // Orch 对用户意图的分析
    SubTasks       []SubTask         `json:"sub_tasks"`        // 拆解的子任务
    Summary        string            `json:"summary"`          // 最终汇总结果
    CreatedAt      time.Time         `json:"created_at"`
    UpdatedAt      time.Time         `json:"updated_at"`
}

type OrchTaskStatus string

const (
    OrchTaskPending     OrchTaskStatus = "pending"      // 等待 Orch 分析
    OrchTaskRunning     OrchTaskStatus = "running"      // 子任务执行中
    OrchTaskAggregating OrchTaskStatus = "aggregating"  // 正在汇总
    OrchTaskDone        OrchTaskStatus = "done"         // 全部完成
    OrchTaskFailed      OrchTaskStatus = "failed"       // 整体失败
)
```

### 3.2 SubTask — 子任务（派给工作 Agent）

```go
// SubTask：Orch 拆解出的子任务，分派给某个 Agent
type SubTask struct {
    ID              string         `json:"id"`
    OrchTaskID      string         `json:"orch_task_id"`
    AgentID         string         `json:"agent_id"`
    AgentName       string         `json:"agent_name"`
    Description     string         `json:"description"`       // 子任务描述（给 Agent 的指令）
    InjectedContext []ContextChunk `json:"injected_context"`  // Orch 选择注入的上下文
    Status          SubTaskStatus  `json:"status"`
    Result          string         `json:"result"`            // Agent 返回结果
    Error           string         `json:"error,omitempty"`   // 失败原因
    DaemonTaskID    string         `json:"daemon_task_id"`    // 关联的 DaemonTask ID
    StartedAt       *time.Time     `json:"started_at,omitempty"`
    CompletedAt     *time.Time     `json:"completed_at,omitempty"`
}

type SubTaskStatus string

const (
    SubTaskPending   SubTaskStatus = "pending"    // 等待分派
    SubTaskDispatched SubTaskStatus = "dispatched" // 已通过 MCP 派出
    SubTaskRunning   SubTaskStatus = "running"    // Agent 执行中
    SubTaskDone      SubTaskStatus = "done"       // 完成
    SubTaskFailed    SubTaskStatus = "failed"     // 失败
)
```

### 3.3 ContextChunk — 注入上下文片段

```go
// ContextChunk：Orch 从群聊历史中选择的上下文片段
type ContextChunk struct {
    Type     ContextType `json:"type"`      // message/kb_file/code/artifact
    SourceID string      `json:"source_id"` // 原始消息/文件 ID
    Content  string      `json:"content"`   // 实际内容
    Summary  string      `json:"summary"`   // 可选的一句话摘要
}

type ContextType string

const (
    ContextMessage  ContextType = "message"   // 群聊历史消息
    ContextKBFile   ContextType = "kb_file"   // 知识库文件
    ContextCode     ContextType = "code"      // 代码片段
    ContextArtifact ContextType = "artifact"  // Agent 产物
)
```

### 3.4 前端事件（WS 推送）

```typescript
// 后端 → 前端的 WS 事件
interface OrchEvent {
  type:
    | "orch.task_created"     // 编排开始
    | "orch.analysis"         // 意图分析完成
    | "orch.subtask_dispatched" // 子任务已派出
    | "orch.subtask_progress"  // 子任务进度更新
    | "orch.subtask_done"      // 单个子任务完成
    | "orch.subtask_failed"    // 单个子任务失败
    | "orch.aggregating"       // 开始汇总
    | "orch.done"              // 全部完成
    | "orch.failed";           // 整体失败
  data: OrchestratorTask;
}
```

---

## 四、核心流程

```
用户在群聊发消息
       │
       ▼
┌─────────────────────┐
│ 1. Orch 接收消息     │  MessageService 检测到群聊 + 无 @mention
│    构建群聊上下文     │  → 调用 OrchestratorService.HandleGroupMessage()
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 2. 意图分析          │  调用 Orch Agent (Claude Code CLI)
│    + 任务拆解        │  System Prompt 包含：群成员列表 + 最近 N 条消息 + 可用 Agent 列表
│                      │  LLM 输出结构化 JSON: { analysis, subtasks[] }
│    → 创建 OrchTask   │
│    → WS 推送 orch.task_created + orch.analysis
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 3. 上下文选择        │  Orch 根据子任务描述，从群聊历史中选择相关上下文
│    + 子任务组装       │  每个子任务只注入与其相关的消息/文件
│                      │  构建 SubTask，填充 InjectedContext
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 4. 并行派活 (MCP)    │  对每个 SubTask：
│                      │    a. 调用 daemon MCP → 创建 DaemonTask
│                      │    b. WS 推送 orch.subtask_dispatched
│                      │    c. 任务面板 Sync: 显示 "Agent X 正在处理..."
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 5. 等待 Agent 完成   │  监听 DaemonTask 完成回调
│    + 实时状态同步     │  每个子任务完成时：
│                      │    a. WS 推送 orch.subtask_done
│                      │    b. 任务面板 Sync: 更新子任务状态
│                      │    c. 聊天窗口: 显示 Agent 结果卡片
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 6. 结果汇总          │  所有子任务完成后：
│    + Orch 总结       │    a. WS 推送 orch.aggregating
│                      │    b. 调用 Orch Agent 汇总所有子任务结果
│                      │    c. 生成最终 Summary
│                      │    d. WS 推送 orch.done
│                      │    e. 任务面板 Sync: 显示最终结果
│                      │    f. 发送 Orch 汇总消息到聊天
└─────────────────────┘
```

---

## 五、子任务拆分

### Phase 1: 数据模型 + 存储层（~1 天）

| 任务 | 说明 | 产出 |
|------|------|------|
| 1.1 DB 表设计 | `orchestrator_tasks` + `orchestrator_sub_tasks` 表 | migration SQL |
| 1.2 Model 定义 | Go struct + 常量 | `model/orchestrator.go` |
| 1.3 Repository | CRUD + 按 conversationID 查询 + 状态更新 | `repository/orchestrator.go` |
| 1.4 API 端点 | `GET /api/orchestrator/tasks?conversation_id=xxx` 查询编排任务 | handler + service |

### Phase 2: Orchestrator 意图分析（~1.5 天）

| 任务 | 说明 | 产出 |
|------|------|------|
| 2.1 Orch System Prompt | 设计 Orch 身份 prompt + 可用 Agent 列表 + 输出格式约束 | `orchestrator_prompt.go` 扩展 |
| 2.2 群聊上下文构建 | 拉取最近 N 条群消息 + Agent 列表 + 群成员 → 组装 context | `BuildGroupContext()` |
| 2.3 意图分析调用 | 调用 Orch Agent(Claude Code) 分析意图，解析 JSON 输出 | `AnalyzeIntent()` |
| 2.4 上下文选择算法 | 根据子任务描述，从群聊历史中检索相关消息/文件 | `SelectContext()` |
| 2.5 单元测试 | 意图分析 mock + JSON 解析 + 上下文选择 | `orchestrator_test.go` |

### Phase 3: MCP 派活 + 状态同步（~1.5 天）面向智能体架构自动搜索的企业级协同平台技术研发

| 任务 | 说明 | 产出 |
|------|------|------|
| 3.1 DispatchSubTask | 创建 DaemonTask，通过 MCP 调用 agent | `DispatchSubTask()` |
| 3.2 并行调度 | goroutine 并行派活，context 取消支持 | `DispatchAll()` |
| 3.3 DaemonTask → SubTask 映射 | daemon 完成/失败回调映射到 SubTask 状态更新 | 回调 hook |
| 3.4 WS 推送事件 | 推送 orch.* 系列事件到前端 | `pushOrchEvent()` |
| 3.5 任务面板 Sync | 调用 `workspace_tasks` 表同步状态 | `SyncToTaskPanel()` |

### Phase 4: 结果汇总（~1 天）

| 任务 | 说明 | 产出 |
|------|------|------|
| 4.1 汇总 Prompt | 设计结果汇总 prompt，传入所有子任务结果 | `BuildAggregationPrompt()` |
| 4.2 汇总调用 | Orch Agent 汇总，生成最终 summary | `AggregateResults()` |
| 4.3 最终消息 | 汇总结果作为 Orch Agent 消息发回聊天 | `SendOrchSummary()` |
| 4.4 任务面板最终更新 | 更新 workspace_task 为 completed + summary | `CompleteTaskPanel()` |
| 4.5 错误降级 | 部分 Agent 失败时，返回已完成部分 + 失败说明 | 降级逻辑 |

### Phase 5: 前端 UI（~2 天）

| 任务 | 说明 | 产出 |
|------|------|------|
| 5.1 WS 事件处理 | 前端监听 orch.* 事件，更新 Zustand store | `wsStore.ts` 扩展 |
| 5.2 编排状态卡片 | 聊天中显示编排进度卡片（分析中→派活→执行→汇总） | `OrchestratorCard.tsx` |
| 5.3 子任务进度 | 卡片内展示各子任务状态（Agent 名 + 状态 + 结果预览） | `SubTaskProgress.tsx` |
| 5.4 任务面板同步 | 任务面板实时显示编排任务和子任务进度 | `TaskPanel` 扩展 |
| 5.5 结果展示 | 汇总结果展示：Markdown 渲染 + 子任务结果折叠 | `OrchResultCard.tsx` |
| 5.6 手动触发 | 群聊中 @Orchestrator 或按钮触发编排 | 触发机制 |

---

## 六、Orch System Prompt 设计要点

```
你是 Orchestrator，一个群聊中的任务协调 Agent。

你的职责：
1. 分析用户在群聊中的请求
2. 将复杂任务拆解为可并行的子任务
3. 为每个子任务选择最合适的 Agent
4. 选择性地注入相关群聊上下文给各 Agent

输出格式（严格 JSON）：
{
  "analysis": "对用户请求的理解和分析",
  "subtasks": [
    {
      "id": "st-1",
      "description": "子任务的详细描述",
      "agent": "claude-code",
      "reason": "选择此 Agent 的原因",
      "context_keywords": ["关键词1", "关键词2"]  // 用于上下文检索
    }
  ]
}

可用 Agent 列表：
{{.AgentList}}

群聊最近消息：
{{.RecentMessages}}

群成员：
{{.Members}}
```

---

## 七、存储方案

### OrchestratorTask 存储

- **主存储**: PostgreSQL（持久化，支持查询）
- **实时状态**: Redis（可选，高频更新场景缓存状态）
- **内存**: `activeOrchs` map 已有，用于并发保护

### 数据库 Migration

```sql
CREATE TABLE orchestrator_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    user_id UUID NOT NULL REFERENCES users(id),
    trigger_msg_id UUID REFERENCES messages(id),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    analysis TEXT,
    summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE orchestrator_sub_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    orch_task_id UUID NOT NULL REFERENCES orchestrator_tasks(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id),
    description TEXT NOT NULL,
    injected_context JSONB DEFAULT '[]',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    result TEXT,
    error TEXT,
    daemon_task_id UUID,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_orch_tasks_conv ON orchestrator_tasks(conversation_id);
CREATE INDEX idx_orch_tasks_status ON orchestrator_tasks(status);
CREATE INDEX idx_orch_sub_tasks_orch ON orchestrator_sub_tasks(orch_task_id);
```

---

## 八、与现有代码的关系

| 现有模块 | 变更 |
|----------|------|
| `OrchestratorService` | 新增 `HandleGroupMessage()`、`AnalyzeIntent()`、`DispatchAll()`、`AggregateResults()` |
| `MessageService` | `asyncAgentReply()` 增加群聊编排分支 |
| `DaemonTask` | 无需改动，SubTask 通过 DaemonTask 执行 |
| `workspace_tasks` | 复用现有表，增加 `orch_task_id` 关联字段 |
| WS Hub | 新增 orch.* 事件推送 |
| 前端 store | 新增 `orchStore.ts` 管理编排状态 |

---

## 九、里程碑时间线

| 阶段 | 预计时间 | 依赖 |
|------|----------|------|
| Phase 1: 数据模型 | 1 天 | 无 |
| Phase 2: 意图分析 | 1.5 天 | Phase 1 |
| Phase 3: MCP 派活 | 1.5 天 | Phase 2 |
| Phase 4: 结果汇总 | 1 天 | Phase 3 |
| Phase 5: 前端 UI | 2 天 | Phase 3（可并行） |
| **总计** | **~7 天** | |

---

## 十、验收标准

- [ ] 用户在群聊发消息（不 @特定 Agent），Orch 自动分析意图
- [ ] Orch 拆解任务为 2+ 个子任务，分派给不同 Agent
- [ ] 任务面板实时显示编排状态和各子任务进度
- [ ] 所有 Agent 完成后，Orch 生成汇总结果
- [ ] 聊天窗口展示完整的编排过程（分析→派活→执行→汇总）
- [ ] 部分 Agent 失败时，已有结果仍能正常展示
- [ ] 前端可查询历史编排任务和结果

---

## 十一、MCP 工具扩展

> 目标：让 Agent（通过 daemon）能以 MCP 工具方式操作任务面板、查询机器上的智能体、发送消息等，使 Agent 成为平台的"一等公民"。

### 11.1 现有 MCP 工具

| 工具 | 说明 |
|------|------|
| `agent_list` | 获取用户所有 Agent 列表 |
| `agent_create` | 创建新 Agent |
| `agent_update` | 更新 Agent 配置 |
| `agent_restart` / `agent_stop` / `agent_delete` | Agent 生命周期管理 |
| `machine_list` / `machine_create` / `machine_connect` | 机器管理 |
| `kb_read_file` | 读取知识库文件 |

### 11.2 新增 MCP 工具

#### 任务面板操作

| 工具名 | 方法 | 说明 | 参数 |
|--------|------|------|------|
| `task_list` | `GET /api/tasks` | 查询任务列表 | `conversation_id`(可选), `status`(可选) |
| `task_create` | `POST /api/tasks` | 创建任务卡片 | `title`, `description`, `status`(默认 todo), `conversation_id`(可选) |
| `task_update` | `PUT /api/tasks/:id` | 更新任务内容 | `task_id`, `title`(可选), `description`(可选) |
| `task_move` | `POST /api/tasks/:id/move` | 移动任务状态 | `task_id`, `status`(todo/in_progress/done/cancelled) |
| `task_delete` | `DELETE /api/tasks/:id` | 删除任务 | `task_id` |

#### 智能体 & 机器查询

| 工具名 | 方法 | 说明 | 参数 |
|--------|------|------|------|
| `machine_agents` | `GET /api/daemon/machines/:id/agents` | 查询指定机器上的智能体 | `machine_id` |
| `agent_candidates` | `GET /api/agents/candidates` | 查询可添加的 Agent 候选（已扫描到的 CLI） | 无 |
| `agent_detail` | `GET /api/agents/:id` | 获取 Agent 详情（含 capabilities、tools） | `agent_id` |

#### 对话 & 消息操作

| 工具名 | 方法 | 说明 | 参数 |
|--------|------|------|------|
| `conversation_info` | `GET /api/conversations/:id` | 获取对话详情（含成员列表） | `conversation_id` |
| `conversation_members` | `GET /api/conversations/:id/members` | 获取对话成员列表 | `conversation_id` |
| `message_send` | `POST /api/conversations/:id/messages` | Agent 主动发送消息到对话 | `conversation_id`, `content`, `reply_to`(可选) |
| `message_search` | `GET /api/conversations/:id/messages/search` | 搜索对话历史消息 | `conversation_id`, `keyword` |

#### 编排任务查询（Phase 1 后可用）

| 工具名 | 方法 | 说明 | 参数 |
|--------|------|------|------|
| `orch_task_list` | `GET /api/orchestrator/tasks` | 查询编排任务 | `conversation_id` |
| `orch_task_detail` | `GET /api/orchestrator/tasks/:id` | 编排任务详情（含子任务） | `task_id` |

### 11.3 工具注入方式

现有方式是 `GenerateManagementTools()` 生成 curl 命令作为 Agent 的工具定义。新增工具沿用同样模式：

```go
// agent_tools.go 扩展
func GenerateTaskTools(serverURL, token string) string {
    // 生成 task_* 系列工具的 curl 命令
}

func GenerateConversationTools(serverURL, token string) string {
    // 生成 conversation_* / message_* 系列工具
}

func GenerateOrchTools(serverURL, token string) string {
    // 生成 orch_* 系列工具
}
```

注入时机：
- **所有 Agent**：`task_*`、`conversation_info`、`message_send`
- **Orchestrator Agent**：额外注入 `orch_*`、`machine_agents`
- **按需注入**：`kb_read_file` 已有按需逻辑，新工具同理

### 11.4 MCP 工具扩展任务拆分

| 任务 | 说明 | 预计时间 |
|------|------|----------|
| 后端 API 补全 | `machine_agents`、`agent_candidates`、`agent_detail` 端点 | 0.5 天 |
| 工具定义生成 | `GenerateTaskTools()` + `GenerateConversationTools()` + `GenerateOrchTools()` | 0.5 天 |
| 工具注入逻辑 | 根据场景选择注入哪些工具（所有 Agent vs Orch） | 0.3 天 |
| 权限控制 | Agent JWT token scope 细化（`task:read`、`task:write`、`message:send`） | 0.3 天 |
| 测试 | 端到端测试：Agent 通过工具操作任务面板 | 0.4 天 |
| **总计** | | **~2 天** |

### 11.5 JWT Token Scope 设计

现有 Agent token scope 是 `agent_management`（一个 scope 包含所有操作）。扩展后拆分为：

| Scope | 权限 |
|-------|------|
| `agent:read` | 查询 Agent 列表和详情 |
| `agent:write` | 创建/更新/删除 Agent |
| `task:read` | 查询任务列表 |
| `task:write` | 创建/更新/删除/移动任务 |
| `message:read` | 查询消息和对话 |
| `message:write` | 发送消息 |
| `machine:read` | 查询机器和机器上的 Agent |
| `orch:read` | 查询编排任务 |

- 普通 Agent token: `agent:read` + `task:read` + `task:write` + `message:read` + `message:write`
- Orchestrator token: 全部 scope

---

## 十二、产物内联预览（Artifacts）

> Agent 的回复不仅是文字，还可以内联展示代码 Diff、网页预览卡片、文件附件等富媒体产物，用户可直接在聊天流中预览和操作。

### 12.1 产物类型

| 类型 | 标识 | 说明 | 预览方式 |
|------|------|------|----------|
| 代码 Diff | `code_diff` | Agent 生成的代码变更 | Diff 高亮渲染，支持折叠/展开 |
| 网页预览 | `web_preview` | Agent 生成的 HTML 页面 | iframe 内嵌预览，支持全屏 |
| 文件附件 | `file` | Agent 上传/生成的文件 | 文件卡片 + 下载链接 |
| 图片 | `image` | Agent 生成的图片 | 缩略图 + 点击放大 |
| Markdown 文档 | `markdown` | 长文档、报告 | Markdown 渲染，支持目录跳转 |
| 命令行输出 | `cli_output` | CLI 执行结果 | 终端风格渲染 |

### 12.2 产物数据结构

```go
// MessageArtifact 附加在消息上的产物
type MessageArtifact struct {
    ID        string            `json:"id"`
    MessageID string            `json:"message_id"`
    Type      string            `json:"type"`       // code_diff/web_preview/file/image/markdown/cli_output
    Title     string            `json:"title"`      // 产物标题
    Content   string            `json:"content"`    // 产物内容（代码/HTML/Markdown）
    URL       string            `json:"url"`        // 可选：外部资源 URL
    Meta      json.RawMessage   `json:"meta"`       // 类型相关的元数据
    CreatedAt time.Time         `json:"created_at"`
}

// 各类型的 Meta 定义
type CodeDiffMeta struct {
    Language string `json:"language"`   // 编程语言
    FilePath string `json:"file_path"` // 文件路径
    AddLines int    `json:"add_lines"`
    DelLines int    `json:"del_lines"`
}

type WebPreviewMeta struct {
    Width  int `json:"width"`   // 预览宽度
    Height int `json:"height"`  // 预览高度
}

type FileMeta struct {
    FileName string `json:"file_name"`
    FileSize int64  `json:"file_size"`
    MimeType string `json:"mime_type"`
}
```

### 12.3 产物嵌入协议

Agent 在回复消息中通过特定标记嵌入产物：

```
这是我的分析结果：

:::artifact{id="art-1" type="code_diff" title="修改 main.go"}
diff --git a/main.go ...
:::end

:::artifact{id="art-2" type="web_preview" title="预览页面"}
<html>...</html>
:::end
```

后端解析流程：
1. 消息入库时，提取 `:::artifact{...}...:::end` 块
2. 每个块存为 `message_artifacts` 表记录
3. 消息 `content` 替换为 `:::artifact-ref{id="art-1"}:::` 占位符
4. 前端渲染时，根据 artifact type 选择对应预览组件

### 12.4 前端组件

| 组件 | 说明 |
|------|------|
| `ArtifactCard.tsx` | 通用产物卡片容器（标题栏 + 展开/折叠） |
| `CodeDiffViewer.tsx` | Diff 高亮渲染（基于 react-diff-viewer 或类似库） |
| `WebPreviewFrame.tsx` | iframe 内嵌预览（沙箱隔离） |
| `FileAttachmentCard.tsx` | 文件卡片（图标 + 文件名 + 大小 + 下载） |
| `ImageViewer.tsx` | 图片查看器（缩略图 + Modal 放大） |
| `MarkdownRenderer.tsx` | Markdown 文档渲染（复用现有 react-markdown） |

### 12.5 产物系统任务拆分

| 任务 | 说明 | 预计时间 |
|------|------|----------|
| DB 表设计 | `message_artifacts` 表 + migration | 0.3 天 |
| 产物解析器 | 后端提取 `:::artifact` 块，存入 DB | 0.5 天 |
| API 端点 | `GET /api/messages/:id/artifacts` 查询产物 | 0.2 天 |
| Agent 输出解析 | daemon 返回流中识别产物标记 | 0.5 天 |
| ArtifactCard 组件 | 通用卡片容器 + 类型分发 | 0.5 天 |
| CodeDiffViewer | Diff 渲染组件 | 0.5 天 |
| WebPreviewFrame | iframe 沙箱预览 | 0.3 天 |
| FileAttachmentCard | 文件卡片 + 下载 | 0.3 天 |
| ImageViewer | 图片预览 + 放大 | 0.2 天 |
| 集成测试 | Agent 回复含产物的端到端测试 | 0.2 天 |
| **总计** | | **~3.5 天** |

---

## 十三、总体时间线汇总

| 模块 | 预计时间 | 可并行 |
|------|----------|--------|
| Orchestrator 群聊编排 (Phase 1-5) | ~7 天 | - |
| MCP 工具扩展 (Section 11) | ~2 天 | 与 Phase 1-2 并行 |
| 产物内联预览 (Section 12) | ~3.5 天 | 与 Phase 3-5 并行 |
| **关键路径总计** | **~7 天** | |
