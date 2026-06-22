'use strict';

// ClaudeCliSpec: Claude Code CLI 的 spec 实现。
// 对应原 commandForTask 中 `if (task.cli_tool === 'claude')` 分支（约 :1052-1084）。
// 行为等价要求：args 顺序、--dangerously-skip-permissions vs --permission-mode dontAsk、
// sessionId 优先级（persistent > task._sessionId > makeSessionId）、stdin=userPrompt。

function createClaudeCliSpec(ctx) {
  return {
    // 标识符，用于 Registry 键 + 分发匹配。
    cliTool: 'claude',
    // UI 名（CANDIDATES 派生时使用）。
    name: 'Claude Code',
    // 默认能力（capabilities），当 skill 扫描无结果时回退。
    defaultCapabilities: ctx.defaultSkills(['coding', 'review', 'orchestration']),

    // buildCommand 返回和原 commandForTask claude 分支完全相同的 spec 对象。
    // 字段：{ command, args, stdin, sessionId }
    // 注意：sessionId 仅作为提示传给 executeTask（用于 --resume 重试链），
    // 不在此处拼装到 args（executeTask 会自己加 --resume / --session-id）。
    buildCommand(task, deps) {
      const { command, systemPrompt, userPrompt } = deps;
      const sessionId = task._sessionId
        || (task.conversation_id && task.agent_id
          ? ctx.makeSessionId(task.conversation_id, task.agent_id)
          : null);
      // Persistent agent (registered via agent.start) — sessionId 取自 runningAgents。
      const persistent = task.agent_id && deps.runningAgents.has(task.agent_id);
      // 卡片文件路径来自 TaskContext 的 cards collector，注入到 MCP 子进程，
      // 让 render_card 工具能写回本任务的卡片输出。
      const cardFile = deps.cardFile;

      const args = [
        '-p',
        '--output-format',
        'text',
        '--thinking',
        'off',
        ...ctx.buildPlatformMcpArgs(task.conversation_id, task.user_id, task.agent_id, cardFile),
      ];
      if (persistent) {
        args.push('--dangerously-skip-permissions');
      } else {
        args.push('--permission-mode', 'dontAsk');
      }
      if (systemPrompt) {
        args.push('--system-prompt', systemPrompt);
      }

      const effectiveSessionId = persistent
        ? deps.runningAgents.get(task.agent_id).sessionId
        : sessionId;

      return {
        command,
        args,
        stdin: userPrompt,
        sessionId: effectiveSessionId,
      };
    },

    // Claude 通过 buildPlatformMcpArgs 按任务注入 MCP，无需启动期全局注册。
    // 不实现 ensureMcp。

    // skillRoots 对应原 skillRoots() claude 分支（约 :643-647）。
    // 返回相对/绝对路径数组（已去重，由调用方维护顺序）。
    skillRoots(cwd, home) {
      const roots = [];
      const includeProjectRoots = !ctx.isAgentHubWorkspace(cwd);
      if (includeProjectRoots) ctx.addRoot(roots, ctx.pathJoin(cwd, '.claude', 'skills'));
      if (home) {
        ctx.addRoot(roots, ctx.pathJoin(home, '.claude', 'skills'));
        ctx.addRoot(roots, ctx.pathJoin(home, '.claude', 'plugins', 'marketplaces'));
        ctx.addRoot(roots, ctx.pathJoin(home, '.claude', 'plugins', 'cache'));
      }
      return roots;
    },

    // Claude 无登录态特殊检测（命令存在即视为可用），不实现 isAuthenticated。
  };
}

module.exports = { createClaudeCliSpec };
