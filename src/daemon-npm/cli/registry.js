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
