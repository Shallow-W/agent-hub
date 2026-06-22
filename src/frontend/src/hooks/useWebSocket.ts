import { useEffect, useCallback } from 'react';
import { useWsStore, dispatchWsEvent } from '@/store/wsStore';
import { useMessageStore } from '@/store/messageStore';
import { invalidateMessageCache } from '@/hooks/useMessages';
import { useConversationStore } from '@/store/conversationStore';
import { useAuthStore } from '@/store/authStore';
import { useAgentStore } from '@/store/agentStore';
import { message } from '@/utils/message';
import type { StreamMessage } from '@/types/message';

let audioCtx: AudioContext | null = null;

function playNotificationBeep() {
  try {
    if (!audioCtx) {
      audioCtx = new AudioContext();
    }
    const osc = audioCtx.createOscillator();
    const gain = audioCtx.createGain();
    osc.connect(gain);
    gain.connect(audioCtx.destination);
    osc.frequency.value = 880;
    osc.type = 'sine';
    gain.gain.value = 0.15;
    osc.start();
    gain.gain.exponentialRampToValueAtTime(0.001, audioCtx.currentTime + 0.15);
    osc.stop(audioCtx.currentTime + 0.15);
  } catch {
    // AudioContext may not be available
  }
}

/** Track auto-removal timers for typing users per conversation+user */
const typingTimers: Record<string, ReturnType<typeof setTimeout>> = {};

function scheduleTypingRemove(conversationId: string, userId: string) {
  const key = `${conversationId}:${userId}`;
  if (typingTimers[key]) clearTimeout(typingTimers[key]);
  typingTimers[key] = setTimeout(() => {
    useWsStore.getState().removeTypingUser(conversationId, userId);
    delete typingTimers[key];
  }, 3000);
}

export function useWebSocket() {
  const token = useAuthStore((s) => s.token);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const status = useWsStore((s) => s.status);
  const connect = useWsStore((s) => s.connect);
  const wsClient = useWsStore((s) => s.wsClient);
  const updateStreaming = useMessageStore((s) => s.updateStreaming);
  const appendDeltas = useMessageStore((s) => s.appendDeltas);
  const completeStreaming = useMessageStore((s) => s.completeStreaming);
  const incrementUnread = useMessageStore((s) => s.incrementUnread);
  const addMessage = useMessageStore((s) => s.addMessage);

  useEffect(() => {
    if (!isAuthenticated || !token) return;

    // 连接 WebSocket 并立即注册消息回调
    const client = connect(token);

    const handleMessage = (msg: StreamMessage) => {
      if (!msg.data) return;

      // 泛化事件分发——所有订阅者通过 onWsEvent / useWsEventHandler 接收。
      dispatchWsEvent(msg.type, msg.data);

      const { conversationId, conversation_id, messageId, content } = msg.data;
      const convId = conversationId ?? conversation_id;

      // error 事件不需要 convId，提前处理
      if (msg.type === 'error') {
        const errMsg = msg.data.message || '连接发生错误';
        console.error('WebSocket error:', errMsg);
        message.error(errMsg);
        return;
      }

      if (!convId) return;

      const activeId = useConversationStore.getState().activeConversationId;

      switch (msg.type) {
        case 'message.streaming': {
          // 新路径：daemon 透传 AgentEvent[]，前端按 kind 聚合渲染。
          // 旧路径兼容：仍然会携带 content，但若有 deltas 则优先用 deltas。
          const msgId = msg.data.message_id ?? msg.data.messageId ?? messageId;
          if (msgId && msg.data.deltas && msg.data.deltas.length > 0) {
            appendDeltas(convId, msgId, msg.data.deltas, {
              taskId: msg.data.task_id,
              agentId: msg.data.agent_id,
            });
          } else if (messageId && content) {
            updateStreaming(convId, messageId, content);
          }
          break;
        }
        case 'message.complete': {
          const msgId = msg.data.id ?? messageId;
          const msgContent = msg.data.content;
          if (!msgId || !msgContent) break;

          // Full message push from Hub (has id + conversation_id)
          if (msg.data.id && msg.data.conversation_id) {
            addMessage(convId, {
              id: msg.data.id,
              conversation_id: msg.data.conversation_id,
              role: msg.data.role ?? 'assistant',
              content: msgContent,
              artifacts_json: msg.data.artifacts_json ?? null,
              created_at: msg.data.created_at ?? new Date().toISOString(),
              attachments: msg.data.attachments,
              artifacts: msg.data.artifacts,
              sender_id: msg.data.sender_id,
              username: msg.data.username,
              reply_to: msg.data.reply_to ?? null,
              reply_to_message: msg.data.reply_to_message ?? null,
              // 交互式卡片——Agent 回复通过 render_card 渲染时随消息推送，
              // 免去刷新页面即可在聊天中显示卡片。
              cards_json: msg.data.cards_json,
              cards: msg.data.cards,
            });
          } else if (messageId && content) {
            // Streaming completion
            completeStreaming(convId, messageId, {
              id: messageId,
              conversation_id: convId,
              role: 'assistant',
              content,
              artifacts_json: null,
              created_at: new Date().toISOString(),
            });
          }
          if (convId !== activeId) {
            incrementUnread(convId);
            playNotificationBeep();
          }
          break;
        }
        case 'user.typing_start': {
          const userId = msg.data.userId;
          if (userId && userId !== useAuthStore.getState().user?.id) {
            useWsStore.getState().addTypingUser(convId, userId, msg.data.username);
            scheduleTypingRemove(convId, userId);
          }
          break;
        }
        case 'user.typing_stop': {
          const userId = msg.data.userId;
          if (userId) {
            useWsStore.getState().removeTypingUser(convId, userId);
            const timerKey = `${convId}:${userId}`;
            if (typingTimers[timerKey]) {
              clearTimeout(typingTimers[timerKey]);
              delete typingTimers[timerKey];
            }
          }
          break;
        }
        case 'agent.status': {
          const agentId = msg.data.agent_id;
          const agentStatus = msg.data.agent_status;
          if (agentId && agentStatus) {
            useAgentStore.getState().updateAgentStatus(agentId, agentStatus as import('@/types/agent').AgentStatus);
          }
          break;
        }
        case 'agent.typing_start': {
          useWsStore.getState().setAgentTyping(convId, true);
          break;
        }
        case 'agent.typing_stop': {
          useWsStore.getState().setAgentTyping(convId, false);
          break;
        }
        case 'message.recall': {
          const recallConvId = msg.data.conversation_id ?? msg.data.conversationId;
          const recallMsgId = msg.data.message_id ?? msg.data.messageId;
          if (recallConvId && recallMsgId) {
            useMessageStore.getState().handleRecallPush(recallConvId, recallMsgId);
          }
          break;
        }
        // task.changed 和 conversation.role_changed 已由泛化 pubsub 处理
        // （dispatchWsEvent 在函数顶部统一分发），无需在此 switch 中处理。
        // error 事件已在 convId guard 之前处理。
      }
    };

    client?.onMessage(handleMessage);

    return () => {
      client?.offMessage();
      // 仅清理本组件注册的消息回调，不断开全局 WebSocket。
      // wsStore.connect() 内部会在依赖变化重连时自动断开旧连接。
    };
    // 仅在认证状态变化时重连
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAuthenticated, token]);

  const send = useCallback(
    (message: string) => {
      wsClient?.send(message);
    },
    [wsClient],
  );

  // Invalidate message cache when WS reconnects (picks up missed messages)
  useEffect(() => {
    if (status === 'connected') {
      invalidateMessageCache();
    }
  }, [status]);

  return { status, send };
}
