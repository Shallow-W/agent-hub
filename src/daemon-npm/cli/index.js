'use strict';

// cli/index.js — CliToolSpec 注册表入口。
//
// 主 daemon 文件（agenthub-daemon.js）只需 require 此模块即可：
//   const cliTools = require('../cli');
//   cliTools.initCliTools(depsCtx);  // 注入 daemon 辅助函数后注册 4 个 spec
//
// 这样新增 CLI（如 gemini-cli）只需：
//   1. 新建 cli/gemini.js 导出 createXxxCliSpec(ctx) 工厂
//   2. 在 initCliTools 内加一行 registerCliTool(createXxxCliSpec(ctx))
// 零修改 commandForTask / ensureGlobalMcpConfigs / skillRoots / scanAgents 四个分发函数。

const {
  registerCliTool,
  getCliTool,
  allCliTools,
  clearCliTools,
} = require('./registry');
const { createClaudeCliSpec } = require('./claude');
const { createCodexCliSpec } = require('./codex');
const { createOpenCodeCliSpec } = require('./opencode');
const { createOpenClawCliSpec } = require('./openclaw');

// initCliTools 接收 daemon 上下文（依赖注入），构造并注册全部已知 spec。
// ctx 必须包含 spec 需要的全部辅助函数（见 cli/*.js 中的 ctx.* 引用）。
// 重复调用会先 clear 再注册，便于测试隔离。
function initCliTools(ctx) {
  clearCliTools();
  registerCliTool(createClaudeCliSpec(ctx));
  registerCliTool(createCodexCliSpec(ctx));
  registerCliTool(createOpenCodeCliSpec(ctx));
  registerCliTool(createOpenClawCliSpec(ctx));
}

module.exports = {
  registerCliTool,
  getCliTool,
  allCliTools,
  clearCliTools,
  initCliTools,
};
