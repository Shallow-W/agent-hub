import { useEffect, useCallback } from 'react';
import { useWsStore } from '@/store/wsStore';
import { useMessageStore } from '@/store/messageStore';
import { useAuthStore } from '@/store/authStore';
import type { StreamMessage } from '@/types/message';

export function useWebSocket() {
  const token = useAuthStore((s) => s.token);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const status = useWsStore((s) => s.status);
  const connect = useWsStore((s) => s.connect);
  const disconnect = useWsStore((s) => s.disconnect);
  const wsClient = useWsStore((s) => s.wsClient);
  const updateStreaming = useMessageStore((s) => s.updateStreaming);
  const completeStreaming = useMessageStore((s) => s.completeStreaming);

  useEffect(() => {
    if (!isAuthenticated || !token) return;

    connect(token);

    // 注册消息回调
    const handleMessage = (msg: StreamMessage) => {
      if (!msg.data) return;
      const { conversationId, messageId, content } = msg.data;

      if (!conversationId) return;

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
          break;
        case 'error':
          // TODO: 全局错误提示
          console.error('WebSocket error:', msg.data.message);
          break;
      }
    };

    wsClient?.onMessage(handleMessage);

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
