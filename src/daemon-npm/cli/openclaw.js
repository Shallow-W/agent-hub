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

    // === Step 2 扩展（agent-adapter 重构） ===

    // resolveCommand：OpenClaw 命令名即执行名，走 default 分支。
    resolveCommand(_taskOrCtx) {
      return 'openclaw';
    },

    // parseResult：把 daemon.js parseOpenClawOutput 搬进 spec。
    // 尝试 JSON 解析后按多组字段优先级提取文本；JSON 失败则返回原始 stdout。
    parseResult({ stdout } = {}, _daemonCtx) {
      const text = String(stdout || '').trim();
      if (!text) return '(OpenClaw CLI 没有返回内容)';
      try {
        const parsed = JSON.parse(text);

        if (typeof parsed.finalAssistantVisibleText === 'string' && parsed.finalAssistantVisibleText.trim()) {
          return parsed.finalAssistantVisibleText.trim();
        }
        if (typeof parsed.finalAssistantRawText === 'string' && parsed.finalAssistantRawText.trim()) {
          return parsed.finalAssistantRawText.trim();
        }
        if (Array.isArray(parsed.payloads)) {
          const payloadText = parsed.payloads
            .map((payload) => (typeof payload?.text === 'string' ? payload.text : ''))
            .filter(Boolean)
            .join('\n')
            .trim();
          if (payloadText) return payloadText;
        }
        if (Array.isArray(parsed.messages)) {
          const msgText = parsed.messages
            .filter((m) => typeof m?.content === 'string' && m.role === 'assistant')
            .map((m) => m.content)
            .join('\n')
            .trim();
          if (msgText) return msgText;
        }
        if (typeof parsed.content === 'string' && parsed.content.trim()) {
          return parsed.content.trim();
        }
        for (const key of ['response', 'result', 'output', 'text', 'message']) {
          if (typeof parsed[key] === 'string' && parsed[key].trim()) {
            return parsed[key].trim();
          }
        }
        if (parsed.data && typeof parsed.data === 'object') {
          for (const key of ['text', 'content', 'message', 'response']) {
            if (typeof parsed.data[key] === 'string' && parsed.data[key].trim()) {
              return parsed.data[key].trim();
            }
          }
        }
      } catch {
        return text;
      }
      return text;
    },
  };
}

module.exports = { createOpenClawCliSpec };
