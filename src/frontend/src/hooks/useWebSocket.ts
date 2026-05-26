import { useEffect, useCallback } from 'react';
import { useWsStore } from '@/store/wsStore';
import { useMessageStore } from '@/store/messageStore';
import { useConversationStore } from '@/store/conversationStore';
import { useAuthStore } from '@/store/authStore';
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
  const disconnect = useWsStore((s) => s.disconnect);
  const wsClient = useWsStore((s) => s.wsClient);
  const updateStreaming = useMessageStore((s) => s.updateStreaming);
  const completeStreaming = useMessageStore((s) => s.completeStreaming);
  const incrementUnread = useMessageStore((s) => s.incrementUnread);
  const currentUserId = useAuthStore((s) => s.user?.id);

  useEffect(() => {
    if (!isAuthenticated || !token) return;

    // 连接 WebSocket 并立即注册消息回调
    const client = connect(token);

    const handleMessage = (msg: StreamMessage) => {
      if (!msg.data) return;
      const { conversationId, messageId, content } = msg.data;

      if (!conversationId) return;

      const activeId = useConversationStore.getState().activeConversationId;

      switch (msg.type) {
        case 'message.streaming':
          if (messageId && content) {
            updateStreaming(conversationId, messageId, content);
          }
          break;
        case 'message.complete':
          if (messageId && content) {
            // 流式结束，用完整内容生成最终消息
            completeStreaming(conversationId, messageId, {
              id: messageId,
              conversation_id: conversationId,
              role: 'assistant',
              content,
              artifacts_json: null,
              created_at: new Date().toISOString(),
            });
          }
          // If this conversation is not currently active, increment unread and notify
          if (conversationId !== activeId) {
            incrementUnread(conversationId);
            playNotificationBeep();
          }
          break;
        case 'user.typing_start': {
          const userId = msg.data.userId;
          if (userId && userId !== currentUserId) {
            useWsStore.getState().addTypingUser(conversationId, userId);
            scheduleTypingRemove(conversationId, userId);
          }
          break;
        }
        case 'user.typing_stop': {
          const userId = msg.data.userId;
          if (userId) {
            useWsStore.getState().removeTypingUser(conversationId, userId);
            const timerKey = `${conversationId}:${userId}`;
            if (typingTimers[timerKey]) {
              clearTimeout(typingTimers[timerKey]);
              delete typingTimers[timerKey];
            }
          }
          break;
        }
        case 'error':
          // TODO: 全局错误提示
          console.error('WebSocket error:', msg.data.message);
          break;
      }
    };

    client?.onMessage(handleMessage);

    return () => {
      disconnect();
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

  return { status, send };
}
