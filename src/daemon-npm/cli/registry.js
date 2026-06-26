'use strict';

// CliToolSpec: 每个 CLI 工具（claude/codex/opencode/openclaw）都注册一个 spec 实例，
// 把原本散落在 commandForTask / ensureGlobalMcpConfigs / skillRoots / scanAgents
// 里的 4 处分支点（architect 报告反模式 #2）收敛为一个对象，新增 CLI 只需
// 实现 spec + 调用 registerCliTool，零修改分发函数。
//
// 设计原则：
// - 零行为变更：buildCommand 返回值必须和原 commandForTask 分支完全一致
//   （args 顺序、env、cwd、sessionId、resultFormat 等字段一一对应）。
// - 可独立测试：spec 不直接 require 主 daemon 文件（会循环依赖），所有 daemon
//   辅助函数通过 ctx（CliToolContext）依赖注入传入。
// - 可选方法：未实现的钩子（ensureMcp / skillRoots / isAuthenticated）以 undefined
//   表示该 CLI 不参与该分支，分发函数会跳过。
//
// === Step 2 扩展（agent-adapter 重构） ===
// 新增的可选方法（用于 Step 4 切换 call site，当前 Step 2-3 只搬代码不切换）：
//
//   resolveCommand(ctx) -> string
//     返回 CLI 可执行路径。替代 daemon.js 中 resolveCodexCommand / resolveOpenCodeCommand
//     的 switch 分发。未实现则走 default（返回 cliTool 字符串）。
//
//   parseResult({ stdout, stderr, outputFile, meta }, ctx) -> string
//     把 one-shot 进程的 stdout/stderr/outputFile 解析成展示文本。
//     替代 daemon.js 中 spec.resultFormat === 'openclaw-json' / 'opencode-json' 的 dispatch。
//     meta.persistSessionKey 由调用方传入（opencode 用来持久化 sessionId）。
//
//   spawnPersistent({ agentId, sessionId, systemPrompt, resume, conversationId, userId,
//                     taskCtx }, ctx) -> { child, sessionId, sendPrompt, events }
//     启动 persistent 进程。events 是 AsyncIterable<AgentEvent>，消费者等 TURN_END
//     即可拿到 turn 结果。目前只有 claude 实现。
//
//   parseStreamEvent(rawLine, ctx) -> AgentEvent | null
//     解析 persistent 进程的一行 stdout 为 AgentEvent，spawnPersistent 内部用。
//     非 JSON 行或不可解析返回 null。
//
// 这些方法在 Step 2-3 阶段是 dormant 的（spec 上有定义但 daemon.js 还没切过去调用）。
// Step 4 会逐个把 daemon.js 的硬编码分支替换为 spec.方法 调用。
//
// === CliStreamAdapter 契约（PR5 正式落地） ===
// 每个 CLI spec 通过实现以下可选方法加入"流式适配器"层（统一 AgentEvent 输出）：
//
//   spec.parseStreamEvent(line, ctx) -> AgentEvent | AgentEvent[] | null
//     解析 CLI 持久进程 stdout 的一行为 AgentEvent。非 JSON 或不可识别返回 null。
//     调用方（spec.spawnPersistent 内部 stdout.on('data') 闭包）批量调用并喂给
//     StreamBuffer，由 daemon 节流后转为 task.progress WS 消息。
//
//   spec.parseStreamEventAll(line, ctx) -> AgentEvent[]
//     multi-event 版本（默认实现：调 parseStreamEvent 并 normalize 为数组）。
//
//   spec.spawnPersistent({ agentId, sessionId, systemPrompt, resume, conversationId,
//                         userId, taskCtx, eventRef }, ctx) -> { child, sessionId, sendPrompt, events }
//     启动 persistent 进程。eventRef 是 { current: fn | null } mutable 引用，
//     dispatcher 把 onEvent 装入 eventRef.current，让 stdout 闭包透明读取。
//     events 是 AsyncIterable<AgentEvent>（备用观察通道）。
//
// 未实现 parseStreamEvent 的 spec 被视为不支持流式（one-shot 模式，走 parseResult 路径）。
// 目前仅 ClaudeCliSpec 完整实现，Codex/OpenCode/OpenClaw 提供占位（返回 null），
// 未来接入时改实现即可，零修改 daemon 主流程。

const CLI_TOOL_REGISTRY = new Map();

// registerCliTool 注册一个 spec 实例，键为 spec.cliTool。
// 重复注册（同 cliTool）会覆盖，便于测试时替换。
function registerCliTool(spec) {
  if (!spec || !spec.cliTool) {
    throw new Error('CliToolSpec must define cliTool');
  }
  CLI_TOOL_REGISTRY.set(spec.cliTool, spec);
}

function getCliTool(cliTool) {
  return CLI_TOOL_REGISTRY.get(cliTool);
}

function allCliTools() {
  return [...CLI_TOOL_REGISTRY.values()];
}

function clearCliTools() {
  CLI_TOOL_REGISTRY.clear();
}

module.exports = {
  CLI_TOOL_REGISTRY,
  registerCliTool,
  getCliTool,
  allCliTools,
  clearCliTools,
};
