'use strict';

// OpenCodeCliSpec: OpenCode CLI 的 spec 实现。
// 对应：
// - commandForTask opencode 分支（约 :1023-1051）：session 复用 / --fork / persistSessionKey
// - registerOpenCodeMcp + ensureOpenCodeMcpConfig（约 :234-270）：启动期全局 MCP 配置文件
// - skillRoots opencode/openclaw 共享分支（约 :651-664）

// extractOpenCodeSessionId/textFromOpenCodeContent/collectOpenCodeText —— 把
// daemon.js 的 parseOpenCodeOutput 辅助函数搬进 spec 模块，让 parseResult 自包含。
function extractOpenCodeSessionId(value) {
  if (!value || typeof value !== 'object') return '';
  for (const key of ['sessionID', 'sessionId', 'session_id']) {
    if (typeof value[key] === 'string' && value[key].trim()) return value[key].trim();
  }
  if (value.session && typeof value.session === 'object') {
    for (const key of ['id', 'sessionID', 'sessionId', 'session_id']) {
      if (typeof value.session[key] === 'string' && value.session[key].trim()) return value.session[key].trim();
    }
  }
  if (value.message && typeof value.message === 'object') {
    return extractOpenCodeSessionId(value.message);
  }
  return '';
}

function textFromOpenCodeContent(content) {
  if (typeof content === 'string') return content;
  if (Array.isArray(content)) {
    return content
      .map((part) => textFromOpenCodeContent(part))
      .filter(Boolean)
      .join('');
  }
  if (!content || typeof content !== 'object') return '';
  if (typeof content.text === 'string') return content.text;
  if (typeof content.content === 'string') return content.content;
  if (Array.isArray(content.parts)) return textFromOpenCodeContent(content.parts);
  return '';
}

function collectOpenCodeText(value, directMessages, partChunks, partUpdates) {
  if (!value || typeof value !== 'object') return;
  const message = value.message && typeof value.message === 'object' ? value.message : value;
  if (message.role === 'assistant') {
    const messageText = textFromOpenCodeContent(message.content || message.parts || message.text);
    if (messageText.trim()) directMessages.push(messageText);
  }
  const part = value.part && typeof value.part === 'object' ? value.part : null;
  if (part && (part.type === 'text' || typeof part.text === 'string')) {
    const text = textFromOpenCodeContent(part);
    if (text.trim()) {
      const partID = String(part.id || value.id || '');
      if (partID && /updated|delta/i.test(String(value.type || ''))) {
        partUpdates.set(partID, text);
      } else {
        partChunks.push(text);
      }
    }
  }
  for (const key of ['response', 'result', 'output', 'text']) {
    if (typeof value[key] === 'string' && value[key].trim() && /assistant|result|complete|response|text/i.test(String(value.type || key))) {
      partChunks.push(value[key]);
    }
  }
}

function createOpenCodeCliSpec(ctx) {
  return {
    cliTool: 'opencode',
    name: 'OpenCode',
    defaultCapabilities: ctx.defaultSkills(['coding']),

    // buildCommand 等价于原 commandForTask opencode 分支。
    // 字段：{ command, args, resultFormat, persistSessionKey, env }
    buildCommand(task, deps) {
      const { command, systemPrompt, userPrompt } = deps;
      const taskId = deps.taskId || task.id || null;
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
        env: ctx.buildAgentHubContextEnv(task.conversation_id, task.user_id, task.agent_id, taskId),
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

    // === Step 2 扩展（agent-adapter 重构） ===

    // resolveCommand：把 daemon.js 中 resolveOpenCodeCommand 的多路径 fallback 搬进来。
    // 等价原行为：
    //   - AGENTHUB_OPENCODE_COMMAND 环境变量优先
    //   - Windows: APPDATA/npm/node_modules/opencode-ai/bin/opencode.exe
    //   - 'opencode' 字面量兜底
    resolveCommand(_taskOrCtx) {
      const candidates = [
        ctx.existingFile(process.env.AGENTHUB_OPENCODE_COMMAND),
        ctx.existingFile(
          process.platform === 'win32' && process.env.APPDATA
            ? ctx.pathJoin(process.env.APPDATA, 'npm', 'node_modules', 'opencode-ai', 'bin', 'opencode.exe')
            : null,
        ),
        'opencode',
      ].filter(Boolean);
      for (const candidate of candidates) {
        if (ctx.commandVersion(candidate) !== null) return candidate;
      }
      return 'opencode';
    },

    // parseResult：把 daemon.js parseOpenCodeOutput 逻辑搬进 spec，并处理 session 持久化副作用。
    // daemonCtx.meta.persistSessionKey 由调用方传入（buildCommand 设置的 spec.persistSessionKey）。
    // 如果 parsed.sessionId 非空且 persistSessionKey 存在，调用 ctx.conversationSessions.set
    // 和 ctx.saveSessionMap —— 等价于 daemon.js executeTask 中的 opencode-json 分支。
    parseResult({ stdout, meta } = {}, _daemonCtx) {
      const text = String(stdout || '').trim();
      if (!text) {
        return { text: '(OpenCode CLI 没有返回内容)', sessionId: '' };
      }

      const events = [];
      const lines = text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
      for (const line of lines) {
        try {
          events.push(JSON.parse(line));
        } catch {
          // OpenCode may fall back to formatted output when JSON is unavailable.
        }
      }
      if (events.length === 0) {
        try {
          events.push(JSON.parse(text));
        } catch {
          return { text, sessionId: '' };
        }
      }

      let sessionId = '';
      const directMessages = [];
      const partChunks = [];
      const partUpdates = new Map();
      for (const event of events) {
        sessionId = sessionId || extractOpenCodeSessionId(event);
        collectOpenCodeText(event, directMessages, partChunks, partUpdates);
      }

      let finalText;
      const directText = directMessages.join('\n').trim();
      if (directText) {
        finalText = directText;
      } else {
        const updatedText = Array.from(partUpdates.values()).join('').trim();
        if (updatedText) {
          finalText = updatedText;
        } else {
          const chunkText = partChunks.join('').trim();
          finalText = chunkText || text;
        }
      }

      // 持久化 session 副作用（原 daemon.js executeTask opencode-json 分支）。
      const persistKey = meta && meta.persistSessionKey;
      if (persistKey && sessionId) {
        ctx.conversationSessions.set(persistKey, sessionId);
        if (typeof ctx.saveSessionMap === 'function') ctx.saveSessionMap();
      }

      return { text: finalText, sessionId };
    },

    // parseStreamEvent / parseStreamEventAll：占位（PR5留）。
    // OpenCode 目前是 one-shot run --format json，不支持 stream-json persistent 模式。
    // 未来实现 OpenCodeStreamAdapter 时在此补全（待 sst/opencode 流式协议调研）。
    parseStreamEvent(_line, _ctx) { return null; },
    parseStreamEventAll(_line, _ctx) { return []; },
  };
}

module.exports = { createOpenCodeCliSpec };
