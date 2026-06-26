import { useCallback, useEffect, useState } from 'react';
import { getConversationAgents } from '../api/conversation';
import type { ConversationAgent } from '../types/conversation';
import { onConversationRoleChanged } from '../store/wsStore';

/**
 * 按 conversationId 拉取 Agent 列表并订阅 WS role_changed 事件自动刷新。
 * GroupMemberPanel / TaskBoardView 等组件共享的公共逻辑。
 */
export function useConversationAgents(conversationId: string | undefined) {
  const [agents, setAgents] = useState<ConversationAgent[]>([]);
  const [loading, setLoading] = useState(false);

  const refetch = useCallback(async () => {
    if (!conversationId) return;
    setLoading(true);
    try {
      const data = await getConversationAgents(conversationId);
      setAgents(data);
    } finally {
      setLoading(false);
    }
  }, [conversationId]);

  useEffect(() => {
    refetch();
  }, [refetch]);

  useEffect(() => {
    if (!conversationId) return;
    return onConversationRoleChanged((payload) => {
      if (payload.conversationId === conversationId) refetch();
    });
  }, [conversationId, refetch]);

  return { agents, loading, refetch };
}
