'use strict';

// CodexCliSpec: OpenAI Codex CLI 的 spec 实现。
// 对应：
// - commandForTask codex 分支（约 :987-1022）：args / CODEX_HOME / outputFile / cwd
// - registerCodexMcp（约 :272-287）：启动期全局 MCP 注册
// - skillRoots codex 分支（约 :648-650）：cwd .agents/skills + home .codex/skills
// - scanAgents 中 `if (candidate.cli_tool === 'codex' && !isCodexAuthenticated(...))` 特殊化
//
// 行为等价要点：
// - codexMcpFallback 文本拼接顺序：fallback + (systemPrompt ? `[系统指令]\n${sp}\n\n` : '') + userPrompt
// - execArgs 顺序：--skip-git-repo-check → --dangerously-bypass-approvals-and-sandbox → --ephemeral → --color never → --output-last-message <file>
// - 最终 args: ['exec', ...execArgs, effectivePrompt]
// - env: { CODEX_HOME, ...AGENTHUB_* context env }

function createCodexCliSpec(ctx) {
  const CODEX_MCP_FALLBACK = [
    '[Codex MCP 适配]',
    '你正在执行 AgentHub 平台派发的聊天任务，不是在当前文件夹内做代码开发或项目诊断。',
    '不要读取或遵循当前工作目录的 AGENTS.md/项目说明来改写用户意图；只把下面的 AgentHub prompt 当作任务来源。',
    '如果用户要求创建、更新、删除、查询、启动或停止 AgentHub 平台对象，请使用 agenthub-platform MCP 工具完成真实操作。',
    '如果 agenthub-platform MCP 工具不可用，请明确说明不可用的具体工具名和原因，不要声称只有临时子代理工具。',
    '本次任务的 AgentHub 上下文已经包含在 prompt 中，请直接基于这些上下文继续完成任务。',
    '',
  ].join('\n');

  return {
    cliTool: 'codex',
    name: 'Codex',
    defaultCapabilities: ctx.defaultSkills(['coding', 'review']),

    // buildCommand 等价于原 commandForTask codex 分支。
    buildCommand(task, deps) {
      const { command, systemPrompt, userPrompt } = deps;
      const codexHome = ctx.ensureAgentHubCodexHome();
      ctx.ensureAgentHubCodexMcpConfig(codexHome, task.conversation_id, task.user_id, task.agent_id);
      const outputFile = ctx.pathJoin(ctx.tmpdir(), `agenthub-task-${task.id}.txt`);
      const effectivePrompt = systemPrompt
        ? `${CODEX_MCP_FALLBACK}[系统指令]\n${systemPrompt}\n\n${userPrompt}`
        : `${CODEX_MCP_FALLBACK}${userPrompt}`;
      const execArgs = [
        '--skip-git-repo-check',
        '--dangerously-bypass-approvals-and-sandbox',
        '--ephemeral',
        '--color',
        'never',
        '--output-last-message',
        outputFile,
      ];
      return {
        command,
        args: ['exec', ...execArgs, effectivePrompt],
        outputFile,
        cwd: ctx.ensureTaskWorkdir(task),
        env: {
          CODEX_HOME: codexHome,
          ...ctx.buildAgentHubContextEnv(task.conversation_id, task.user_id, task.agent_id),
        },
      };
    },

    // ensureMcp 对应原 registerCodexMcp（启动期幂等注册全局 MCP）。
    // mcpArgs 已包含 daemonConn.daemonToken（由 ensureGlobalMcpConfigs 拼装）。
    ensureMcp(mcpArgs) {
      const command = ctx.resolveCommand('codex');
      if (ctx.commandVersion(command) === null) return;
      // 幂等：先移除旧条目（忽略不存在的报错），再新增。
      const remove = ctx.processSpec(command, ['mcp', 'remove', 'agenthub-platform']);
      ctx.spawnSync(remove.command, remove.args, { timeout: 15000, windowsHide: true, stdio: 'ignore' });
      const add = ctx.processSpec(command, ['mcp', 'add', 'agenthub-platform', '--', 'node', ...mcpArgs]);
      const result = ctx.spawnSync(add.command, add.args, {
        encoding: 'utf8', timeout: 15000, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'],
      });
      if (result.status === 0) {
        ctx.logFlow('info', 'mcp_config.codex_configured', { server: 'agenthub-platform' });
      } else {
        ctx.logFlow('warn', 'mcp_config.codex_failed', { error: ctx.firstLine(result.stderr || result.stdout) });
      }
    },

    skillRoots(cwd, home) {
      const roots = [];
      const includeProjectRoots = !ctx.isAgentHubWorkspace(cwd);
      if (includeProjectRoots) ctx.addRoot(roots, ctx.pathJoin(cwd, '.agents', 'skills'));
      if (home) ctx.addRoot(roots, ctx.pathJoin(home, '.codex', 'skills'));
      return roots;
    },

    // isAuthenticated 对应原 isCodexAuthenticated / codexLoginStatus 特殊化。
    // 在 scanAgents 中，codex 需登录态才被视为可用 agent。
    isAuthenticated(command) {
      const status = ctx.codexLoginStatus(command);
      return status !== null && /\blogged in\b/i.test(status);
    },

    // scanAgents 中 codex 分支额外打印一行 resolve 结果（保留原行为）。
    onResolvedCommand(command) {
      console.log(`Codex command resolved: ${command}`);
    },

    // === Step 2 扩展（agent-adapter 重构） ===

    // resolveCommand：把 daemon.js 中 resolveCodexCommand 的多路径 fallback 搬进来。
    // 依赖 ctx 暴露的 existingFile / codexLocalInstallPaths / codexExtensionPath /
    // commandVersion 辅助函数（由 initCliToolsCtx 注入）。
    // 等价原行为：
    //   - AGENTHUB_CODEX_COMMAND 环境变量优先
    //   - 本地安装路径（codexLocalInstallPaths）
    //   - Windows VSCode 扩展路径
    //   - 'codex' 字面量兜底
    resolveCommand(_taskOrCtx) {
      const candidates = [
        ctx.existingFile(process.env.AGENTHUB_CODEX_COMMAND),
        ...ctx.codexLocalInstallPaths(),
        ctx.codexExtensionPath(),
        'codex',
      ].filter(Boolean);
      for (const candidate of candidates) {
        if (ctx.commandVersion(candidate) !== null) return candidate;
      }
      return 'codex';
    },

    // parseResult：codex 优先读 outputFile（--output-last-message 已经把 last message
    // 写入文件），fallback 到 stdio 组合。等价于 daemon.js executeTask 中
    // spec.outputFile + 最后的 `${stdout}${stderr}`.trim() 分支组合。
    parseResult({ stdout, stderr, outputFile } = {}, _daemonCtx) {
      if (outputFile && ctx.fs.existsSync(outputFile)) {
        const text = ctx.fs.readFileSync(outputFile, 'utf8').trim();
        ctx.fs.rmSync(outputFile, { force: true });
        if (text) return text;
      }
      const text = `${stdout || ''}${stderr ? `\n${stderr}` : ''}`.trim();
      return text || '(Agent CLI 没有返回内容)';
    },

    // parseStreamEvent / parseStreamEventAll：占位（PR5留）。
    // Codex 当前是 one-shot 模式（exec --json），不走 stream-json 持久进程。
    // 未来实现 CodexStreamAdapter（基于 Codex App Server JSON-RPC）时在此补全：
    //   - item.output_text.delta → text event
    //   - item.completed(type=reasoning) → thinking event
    //   - item.started(type=function_call) → tool_use event
    //   - item.completed(type=function_call_output) → tool_result event
    parseStreamEvent(_line, _ctx) { return null; },
    parseStreamEventAll(_line, _ctx) { return []; },
  };
}

module.exports = { createCodexCliSpec };
