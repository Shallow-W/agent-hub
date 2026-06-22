'use strict';

// ClaudeCliSpec: Claude Code CLI 的 spec 实现。
// 对应原 commandForTask 中 `if (task.cli_tool === 'claude')` 分支（约 :1052-1084）。
// 行为等价要求：args 顺序、--dangerously-skip-permissions vs --permission-mode dontAsk、
// sessionId 优先级（persistent > task._sessionId > makeSessionId）、stdin=userPrompt。

const {
  EVENT_TYPES,
  textEvent,
  thinkingEvent,
  toolUseEvent,
  turnEndEvent,
  errorEvent,
} = require('./events');

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
      // taskId 注入 MCP subprocess，让 ctx.emitCard 能把工具产出的卡片（如
      // deploy_project info 卡）推到后端 TaskCardQueue。persistent 路径在
      // spawnPersistent 里另行注入（见下方 spawnPersistent 方法）。
      const taskId = deps.taskId || null;

      const args = [
        '-p',
        '--output-format',
        'text',
        '--thinking',
        'off',
        ...ctx.buildPlatformMcpArgs(task.conversation_id, task.user_id, task.agent_id, taskId),
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

    // === Step 2 扩展（agent-adapter 重构） ===
    // 以下方法目前 dormant —— spec 上有定义但 daemon.js 还没切过去调用。
    // Step 4 会把 daemon.js 的对应分支替换为下面的 spec.方法 调用。

    // resolveCommand：Claude 走 default 分支（命令名 === cliTool）。
    // 等价于原 daemon.js resolveCommand('claude') 中最后的 `return cliTool`。
    resolveCommand(_taskOrCtx) {
      return ctx.resolveCommand('claude');
    },

    // parseResult：one-shot 模式的 stdout/stderr fallback。
    // 等价于 daemon.js executeTask 末尾的 `${stdout || ''}${stderr ? `\n${stderr}` : ''}`.trim()。
    parseResult({ stdout, stderr } = {}, _ctx) {
      const text = `${stdout || ''}${stderr ? `\n${stderr}` : ''}`.trim();
      return text || '(Agent CLI 没有返回内容)';
    },

    // parseStreamEvent：解析 Claude stream-json 一行 stdout 为 AgentEvent。
    // 非可识别事件返回 null（调用方忽略）。
    //
    // Claude stream-json 事件类型（v1 观察）：
    //   { type: 'assistant', message: { content: [{ type: 'text', text }, { type: 'tool_use', name, input }, { type: 'thinking', thinking }] } }
    //   { type: 'result', result, is_error, subtype }
    //   { type: 'tool_result', ... } (stream-json 里较少见，通常嵌入 assistant 事件)
    parseStreamEvent(line, _ctx) {
      if (!line || !line.trim()) return null;
      let event;
      try {
        event = JSON.parse(line);
      } catch {
        return null;
      }
      if (!event || typeof event !== 'object') return null;

      if (event.type === 'assistant') {
        const message = event.message && typeof event.message === 'object' ? event.message : null;
        const content = Array.isArray(message?.content) ? message.content : [];
        const out = [];
        for (const part of content) {
          if (!part || typeof part !== 'object') continue;
          if (part.type === 'text' && typeof part.text === 'string') {
            out.push(textEvent(part.text));
          } else if (part.type === 'thinking' && typeof part.thinking === 'string') {
            out.push(thinkingEvent(part.thinking));
          } else if (part.type === 'tool_use') {
            out.push(toolUseEvent(part.name || '', part.input));
          }
        }
        // 兼容原 daemon spawnStreamJsonProcess: 任何 event.type === 'assistant' 行
        // 都会触发 agentTurnStates.set('active')，即使 content 为空或全是未知 part。
        // 这里保证至少返回一个事件（空 text 事件），让 spawnPersistent 的循环能注册 active。
        if (out.length === 0) {
          out.push(textEvent(''));
        }
        return out;
      }

      if (event.type === 'result') {
        const text = typeof event.result === 'string' ? event.result : JSON.stringify(event.result);
        const isError = Boolean(event.is_error || event.subtype === 'error_during_execution');
        if (isError) {
          return turnEndEvent({
            result: text || '',
            error: text || 'Agent execution failed',
            subtype: event.subtype,
          });
        }
        return turnEndEvent({ result: text || '', subtype: event.subtype });
      }

      // tool_result / system 等其它事件类型当前 stream-json 不发送，忽略。
      return null;
    },

    // parseStreamEventAll：parseStreamEvent 的 multi-event 版本，返回 AgentEvent[]。
    // （parseStreamEvent 内部对 assistant 事件可能产生多个事件，这里展开成数组。）
    parseStreamEventAll(line, daemonCtx) {
      const ev = this.parseStreamEvent(line, daemonCtx);
      if (ev === null) return [];
      return Array.isArray(ev) ? ev : [ev];
    },

    // spawnPersistent：启动 Claude persistent 进程（stream-json 模式）。
    //
    // 这是 daemon.js spawnStreamJsonProcess（:2609-2755）的 pure code move：
    // - 完全相同的 args 顺序
    // - 完全相同的 sessionId 策略（传入 sessionId || randomUUID）
    // - 完全相同的 resume / --session-id 决策
    // - 完全相同的 systemPrompt 注入
    // - 完全相同的 stderr / close 日志
    // - 完全相同的 sendPrompt 串行化（queueTail）
    // - 完全相同的 turn timeout
    //
    // 唯一变化：把 stream 事件解析委托给 parseStreamEventAll，并通过 ctx 注入
    // agentTurnStates / logFlow 让逻辑保持纯函数式。
    //
    // 返回 { child, sessionId, sendPrompt, events }：
    //   - events: AsyncIterable<AgentEvent>，包含每个 assistant turn 的 text/tool_use 事件、
    //     每个 result 事件的 turn_end、close 时的 session_end。消费者用 for-await 拿全部。
    //
    // 注意：调用方仍可像旧 spawnStreamJsonProcess 一样用 sendPrompt().then(...) 等待单个 turn
    // （内部仍然是 resultResolver 串行化）；events 是另一条观察通道，给 dispatcher 把
    // AgentEvent 翻译成现有 WS 消息使用。两条通道共享同一份 stdout 解析。
    spawnPersistent({
      agentId,
      sessionId,
      systemPrompt,
      resume,
      conversationId,
      userId,
      taskCtx,
    } = {}, daemonCtx = ctx) {
      const command = daemonCtx.resolveCommand('claude');
      // Bug 1 fix: taskId 必须从 taskCtx.taskId 提取并传给 buildPlatformMcpArgs，
      // 否则 persistent Claude agent 发不出 MCP 工具卡片（emitCard 路径依赖它查 ctx.taskId）。
      const taskId = (taskCtx && taskCtx.taskId) || null;
      const mcpArgs = daemonCtx.buildPlatformMcpArgs(conversationId, userId, agentId, taskId);
      const effectiveSessionId = sessionId || daemonCtx.crypto.randomUUID();

      const args = [
        '--dangerously-skip-permissions',
        '--output-format', 'stream-json',
        '--input-format', 'stream-json',
        '--verbose',
        ...mcpArgs,
        resume ? '--resume' : '--session-id',
        effectiveSessionId,
      ];
      if (systemPrompt) {
        args.push('--system-prompt', systemPrompt);
      }

      const processedSpec = daemonCtx.processSpec(command, args);
      const child = daemonCtx.spawn(processedSpec.command, processedSpec.args, {
        detached: process.platform !== 'win32',
        stdio: ['pipe', 'pipe', 'pipe'],
        windowsHide: true,
      });
      daemonCtx.logFlow('info', 'agent.process_spawn', {
        agent_id: agentId,
        conversation_id: conversationId,
        user_id: userId,
        command: processedSpec.command,
        args_count: processedSpec.args.length,
        session_id: effectiveSessionId,
        resume,
        mcp_enabled: mcpArgs.length > 0,
        system_prompt_len: typeof systemPrompt === 'string' ? systemPrompt.length : 0,
        pid: child.pid,
      });

      const queue = daemonCtx.createAsyncQueue
        ? daemonCtx.createAsyncQueue()
        : require('./events').createAsyncQueue();
      let stdoutBuf = '';
      let resultResolver = null;

      const emitTextAndTools = (events) => {
        for (const ev of events) queue.push(ev);
      };

      child.stdout.setEncoding('utf8');
      child.stdout.on('data', (chunk) => {
        stdoutBuf += chunk;
        const lines = stdoutBuf.split('\n');
        stdoutBuf = lines.pop();
        for (const line of lines) {
          if (!line.trim()) continue;
          const events = this.parseStreamEventAll(line, daemonCtx);
          if (events.length === 0) continue;
          for (const ev of events) {
            if (ev.type === EVENT_TYPES.TURN_END) {
              // Bug 2 fix: 镜像原 spawnStreamJsonProcess（:2682-2689）的 agent.turn_result
              // logFlow，保留线上观察 agent turn 结果的关键事件。
              // 顺序与原代码一致：先 set idle，再 log，再 resolve。
              daemonCtx.agentTurnStates.set(agentId, 'idle');
              const isError = ev.error !== undefined;
              const turnResultText = isError ? ev.error : (ev.result || '');
              daemonCtx.logFlow(
                isError ? 'warn' : 'info',
                'agent.turn_result',
                {
                  agent_id: agentId,
                  conversation_id: conversationId,
                  session_id: effectiveSessionId,
                  is_error: isError,
                  subtype: ev.subtype,
                  result_len: typeof turnResultText === 'string' ? turnResultText.length : 0,
                },
              );
              // resultResolver 由 sendPrompt 设置；resolve 当前 turn 的 promise。
              if (resultResolver) {
                const r = resultResolver;
                resultResolver = null;
                if (ev.error !== undefined) {
                  r({ error: ev.error });
                } else {
                  r({ result: ev.result || '' });
                }
              }
            } else if (
              ev.type === EVENT_TYPES.TEXT
              || ev.type === EVENT_TYPES.TOOL_USE
              || ev.type === EVENT_TYPES.THINKING
            ) {
              // assistant 事件触发 active 状态（与原 spawnStreamJsonProcess 行为一致：
              // 任何 assistant message —— 哪怕只有 thinking —— 都标记为 active）。
              daemonCtx.agentTurnStates.set(agentId, 'active');
            }
            queue.push(ev);
          }
        }
      });

      child.stderr.setEncoding('utf8');
      child.stderr.on('data', (chunk) => {
        daemonCtx.logFlow('warn', 'agent.stderr', {
          agent_id: agentId,
          conversation_id: conversationId,
          session_id: effectiveSessionId,
          message: daemonCtx.truncateStr(chunk.trim(), 500),
        });
      });

      child.on('close', (code) => {
        const hadPendingTurn = Boolean(resultResolver);
        if (resultResolver) {
          const r = resultResolver;
          resultResolver = null;
          r({ error: `Agent process exited (code=${code})` });
        }
        daemonCtx.agentTurnStates.delete(agentId);
        daemonCtx.logFlow(code === 0 ? 'info' : 'warn', 'agent.process_close', {
          agent_id: agentId,
          conversation_id: conversationId,
          session_id: effectiveSessionId,
          pid: child.pid,
          exit_code: code,
          pending_turn: hadPendingTurn,
        });
        queue.push(require('./events').sessionEndEvent({ code }));
        queue.done();
      });

      let queueTail = Promise.resolve();
      const sendPromptRaw = (prompt) => new Promise((resolve, reject) => {
        if (child.exitCode !== null) {
          reject(new Error('Agent process not running'));
          return;
        }
        resultResolver = resolve;
        daemonCtx.logFlow('info', 'agent.prompt_sent', {
          agent_id: agentId,
          conversation_id: conversationId,
          session_id: effectiveSessionId,
          prompt_len: typeof prompt === 'string' ? prompt.length : 0,
        });
        const msg = JSON.stringify({
          type: 'user',
          message: { role: 'user', content: [{ type: 'text', text: prompt }] },
        });
        child.stdin.write(msg + '\n');
        const timer = setTimeout(() => {
          if (resultResolver === resolve) {
            resultResolver = null;
            daemonCtx.logFlow('error', 'agent.turn_timeout', {
              agent_id: agentId,
              conversation_id: conversationId,
              session_id: effectiveSessionId,
              timeout_ms: daemonCtx.EXEC_TIMEOUT_MS,
            });
            reject(new Error(`Agent task timed out (${Math.round(daemonCtx.EXEC_TIMEOUT_MS / 1000)}s)`));
          }
        }, daemonCtx.EXEC_TIMEOUT_MS);
        timer.unref();
      });

      const sendPrompt = (prompt) => {
        const run = () => sendPromptRaw(prompt);
        queueTail = queueTail.then(run, run);
        return queueTail;
      };

      return { child, sessionId: effectiveSessionId, sendPrompt, events: queue.iter };
    },
  };
}

module.exports = { createClaudeCliSpec };
