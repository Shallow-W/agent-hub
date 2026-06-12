'use strict';

// OpenCodeCliSpec: OpenCode CLI 的 spec 实现。
// 对应：
// - commandForTask opencode 分支（约 :1023-1051）：session 复用 / --fork / persistSessionKey
// - registerOpenCodeMcp + ensureOpenCodeMcpConfig（约 :234-270）：启动期全局 MCP 配置文件
// - skillRoots opencode/openclaw 共享分支（约 :651-664）

function createOpenCodeCliSpec(ctx) {
  return {
    cliTool: 'opencode',
    name: 'OpenCode',
    defaultCapabilities: ctx.defaultSkills(['coding']),

    // buildCommand 等价于原 commandForTask opencode 分支。
    // 字段：{ command, args, resultFormat, persistSessionKey, env }
    buildCommand(task, deps) {
      const { command, systemPrompt, userPrompt } = deps;
      const sessionKey = ctx.sessionKeyForTask(task);
      const savedSessionId = sessionKey ? ctx.conversationSessions.get(sessionKey) : '';
      const effectivePrompt = systemPrompt
        ? `[系统指令]\n${systemPrompt}\n\n${userPrompt}`
        : userPrompt;
      const args = [
        'run',
        '--format',
        'json',
        '--no-replay',
        '--dangerously-skip-permissions',
      ];
      if (savedSessionId) {
        args.push('--session', savedSessionId);
        // OpenCode 在 session 内缓存可用工具；上下文变化（agent/conversation 切换）时 fork 让 AgentHub 工具变化生效。
        if (ctx.opencodeContextChanged(task, savedSessionId)) {
          args.push('--fork');
        }
      }
      args.push(effectivePrompt);
      return {
        command,
        args,
        resultFormat: 'opencode-json',
        persistSessionKey: sessionKey,
        env: ctx.buildAgentHubContextEnv(task.conversation_id, task.user_id, task.agent_id),
      };
    },

    // ensureMcp 对应原 registerOpenCodeMcp：写入 opencode.json 的 mcp 配置。
    ensureMcp(mcpArgs) {
      const command = ctx.resolveCommand('opencode');
      if (ctx.commandVersion(command) === null) return;
      try {
        const configPath = ctx.ensureOpenCodeMcpConfig(['node', ...mcpArgs]);
        ctx.logFlow('info', 'mcp_config.opencode_configured', { server: 'agenthub-platform', file: configPath });
      } catch (err) {
        ctx.logFlow('warn', 'mcp_config.opencode_failed', { error: ctx.errorMessage(err) });
      }
    },

    // skillRoots 对应原 skillRoots() opencode/openclaw 共享分支。
    // OpenCode 与 OpenClaw 共享根目录集（保留原行为）。
    skillRoots(cwd, home) {
      const roots = [];
      const includeProjectRoots = !ctx.isAgentHubWorkspace(cwd);
      if (includeProjectRoots) {
        ctx.addRoot(roots, ctx.pathJoin(cwd, '.opencode', 'skills'));
        ctx.addRoot(roots, ctx.pathJoin(cwd, '.openclaw', 'skills'));
      }
      if (home) {
        ctx.addRoot(roots, ctx.pathJoin(home, '.opencode', 'skills'));
        ctx.addRoot(roots, ctx.pathJoin(home, '.openclaw', 'skills'));
        ctx.addRoot(roots, ctx.pathJoin(home, '.openclaw', 'plugin-skills'));
        for (const root of ctx.openClawInstallSkillRoots(home)) {
          ctx.addRoot(roots, root);
        }
      }
      return roots;
    },

    // OpenCode 无登录态特殊检测，不实现 isAuthenticated。
  };
}

module.exports = { createOpenCodeCliSpec };
