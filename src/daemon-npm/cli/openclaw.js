'use strict';

// OpenClawCliSpec: OpenClaw CLI 的 spec 实现。
// 对应：
// - commandForTask openclaw 分支（约 :1085-1104）：agent 子命令、固定 sessionId 回退
// - registerOpenClawMcp（约 :219-232）：启动期 openclaw mcp set
// - skillRoots 与 opencode 共享（在原代码中两者走同一分支，行为等价要求保留）

function createOpenClawCliSpec(ctx) {
  return {
    cliTool: 'openclaw',
    name: 'OpenClaw',
    defaultCapabilities: ctx.defaultSkills(['coding']),

    // buildCommand 等价于原 commandForTask openclaw 分支。
    // 字段：{ command, args, resultFormat }
    // sessionId 回退：task._sessionId → makeSessionId(conv,agent) → agenthub-<sanitized id>
    buildCommand(task, deps) {
      const { command, userPrompt } = deps;
      const sessionId = task._sessionId
        || (task.conversation_id && task.agent_id
          ? ctx.makeSessionId(task.conversation_id, task.agent_id)
          : `agenthub-${String(task.agent_id || task.id).replace(/[^a-zA-Z0-9_-]/g, '-')}`);
      return {
        command,
        args: [
          'agent',
          '--local',
          '--session-id',
          sessionId,
          '--message',
          userPrompt,
          '--json',
          '--thinking',
          'off',
        ],
        resultFormat: 'openclaw-json',
      };
    },

    // ensureMcp 对应原 registerOpenClawMcp：spawn `openclaw mcp set agenthub-platform <json>`。
    ensureMcp(mcpArgs) {
      const command = 'openclaw';
      if (ctx.commandVersion(command) === null) return;
      const value = JSON.stringify({ command: 'node', args: mcpArgs });
      const spec = ctx.processSpec(command, ['mcp', 'set', 'agenthub-platform', value]);
      const result = ctx.spawnSync(spec.command, spec.args, {
        encoding: 'utf8', timeout: 15000, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'],
      });
      if (result.status === 0) {
        ctx.logFlow('info', 'mcp_config.openclaw_configured', { server: 'agenthub-platform' });
      } else {
        ctx.logFlow('warn', 'mcp_config.openclaw_failed', { error: ctx.firstLine(result.stderr || result.stdout) });
      }
    },

    // skillRoots 与 opencode 共享（保留原 if (cliTool === 'opencode' || cliTool === 'openclaw') 行为）。
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

    // OpenClaw 无登录态特殊检测，不实现 isAuthenticated。
  };
}

module.exports = { createOpenClawCliSpec };
