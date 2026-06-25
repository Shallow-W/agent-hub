# 复盘：文件浏览器空白 —— RPC 字段名不匹配导致超时

## 问题
文件浏览器功能（FilesDrawer）开发完成后，端到端测试时：agent 正确生成了 project 卡片（`cards=[{type:"project", workDir:"/Users/shallow/Desktop/tmp"}]`），但用户点击卡片打开抽屉后，**目录树一片空白**，只显示"（未选择）/ 选择左侧文件查看内容"。

## 现象与误导
排查前期在多个错误方向上耗费了时间：

| 怀疑方向 | 实际情况 |
|---|---|
| CSS 高度链断裂导致 antd Tree virtual 不渲染 | 改了 CSS，问题依旧 |
| prompt 没教 agent 怎么用卡片 | 是另一个问题（文本复现），但不是空白的原因 |
| 前端 useEffect / cancelled 竞态 | 不是 |
| 消息 agent_id 为空导致 ProjectCard 不挂载 | artifacts_json 里有 agent_id，不是这个原因 |

**所有前端层面的怀疑都不成立**，因为根本没走到前端渲染那一步。

## 真正的根因
**后端发 `task.dispatch` 时字段名 `"id"` 与 daemon 期望的 `"task_id"` 不匹配**，daemon 拿不到 task id 直接 return，不执行，后端等不到 `task.complete` 响应 → 20 秒超时。

```
后端发:  { "id": "abc-123", "cli_tool": "__agenthub_browse_files__", ... }
daemon 读: data.task_id → undefined
daemon:   if (!task.id) return;   // 直接跳过，不执行
后端:     等 task.complete ... 20s ... 超时 → 50401 浏览文件超时
前端:     catch 超时错误 → 文件树空白
```

涉及两处代码（同样的 bug）：
- `internal/service/agent_browse.go`：`"id": taskID`（文件浏览）
- `internal/service/agent_skill_open.go`：`"id": taskID`（打开 skill 目录，从未被端到端测过所以没暴露）

daemon 端 `handleTaskDispatch`（`agenthub-daemon.js:3000`）读的是 `data.task_id`，全文无 `data.id` 的 fallback。

## 怎么定位的（端到端 curl 测试）
关键转折点：**停止猜前端，直接用 curl 模拟前端调用**。

```bash
# 1. 登录拿 token
TOKEN=$(curl ... /api/auth/login ...)

# 2. curl 直接调 browse API（绕过前端 UI）
curl ".../api/agents/$AGENT_ID/files/browse?action=tree&work_dir=..."

# 返回：{"code":50401,"message":"浏览文件超时","data":null}
```

这一步**一秒锁定问题在 RPC 通道**（后端 → daemon 的 task.dispatch），而不是前端渲染。然后查 daemon 的 task.dispatch handler 读哪个字段 → 发现 `data.task_id`，对比后端发的 `data.id` → 字段名不匹配。

## 修复
后端两处 `"id"` → `"task_id"`：

```go
// 修复前
Data: map[string]interface{}{
    "id": taskID,           // ❌ daemon 读的是 data.task_id
    ...
}

// 修复后
Data: map[string]interface{}{
    "task_id": taskID,      // ✅ 与 daemon 约定一致
    ...
}
```

验证：
```bash
curl browse tree → 200，返回 rootEntries: [App.tsx, index.css, package.json]
curl browse read → 200，返回 package.json 完整内容
```

## 经验与预防措施

### 1. 跨进程契约必须有单一事实源 + 编译期/启动期校验
这次 bug 的本质是**两个独立代码库（Go 后端 + Node daemon）之间没有共享的类型定义**，字段名靠人肉记忆保持一致。`task_id` vs `id` 这种错误，TS/Go 的类型系统都拦不住（因为是 `map[string]interface{}` 松散字典）。

**预防**：
- 跨进程消息字段，在 daemon 端的 WS handler 处加 `assert(data.task_id, 'task.dispatch 缺少 task_id')`（运行时校验，至少让错误信息有意义而不是静默 return）
- 或在 task.dispatch handler 日志里打出收到的完整 data keys，便于发现字段名漂移

### 2. 端到端 curl 测试应作为诊断的第一步
这次在 CSS / prompt / useEffect 上各花了一轮，最后 curl 一秒定位。**对于"功能不工作"类问题，先用 curl 验证 API 层**，确定数据是否到达、后端是否返回正确，再往 UI 层查。

**预防**：遇到"前端空白/不渲染"，第一步永远是打开浏览器 Network 面板看请求响应，或直接 curl 复现。不要从 CSS/渲染开始猜。

### 3. 新增 RPC 调用必须用 curl 端到端验证一次
`agent_browse.go` 是新写的 RPC 调用，写完后没有 curl 验证就直接交付前端联调。`agent_skill_open.go` 更是从来没被端到端测过，所以同样的 bug 藏了很久。

**预防**：任何新增的 `SendToMachine(task.dispatch)` 路径，写完后立刻 curl 测一次（哪怕返回的是业务错误，只要不是"超时"就说明 RPC 通道是通的）。

### 4. 静默 return 是调试的地狱
daemon `handleTaskDispatch` L3008-3010：
```js
if (!task.id) {
  logFlow('warn', 'task.dispatch_invalid', { reason: 'missing task_id' });
  return true;
}
```
这里其实有日志（`task.dispatch_invalid`），但**后端不读 daemon 日志**，后端只看到"超时"。从后端视角，完全无法知道是"daemon 没执行"还是"daemon 执行了但慢"。

**预防**：跨进程 RPC，接收方校验失败时除了本地日志，还应**主动回一个错误响应**（`task.complete` with error），而不是静默 return 让调用方超时。

## 相关代码位置
```
后端发送端（字段名来源）：
  src/backend/internal/service/agent_browse.go        — BrowseAgentFiles SendToMachine
  src/backend/internal/service/agent_skill_open.go    — OpenDaemonSkillLocation SendToMachine

daemon 接收端（字段名期望）：
  src/daemon-npm/bin/agenthub-daemon.js:3000          — handleTaskDispatch: id: data.task_id

task.dispatch 契约（无单一事实源，靠人肉同步）：
  字段: task_id / cli_tool / prompt / agent_id / conversation_id / user_id / context_messages
```
