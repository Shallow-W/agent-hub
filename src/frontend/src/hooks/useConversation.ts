import { useEffect } from 'react';
import { useConversationStore } from '@/store/conversationStore';

export function useConversation() {
  const conversations = useConversationStore((s) => s.conversations);
  const activeId = useConversationStore((s) => s.activeConversationId);
  const loading = useConversationStore((s) => s.loading);
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const createConversation = useConversationStore((s) => s.createConversation);
  const deleteConversation = useConversationStore((s) => s.deleteConversation);
  const togglePin = useConversationStore((s) => s.togglePin);
  const renameConversation = useConversationStore((s) => s.renameConversation);
  const setActive = useConversationStore((s) => s.setActive);

  useEffect(() => {
    const { conversations, loading } = useConversationStore.getState();
    if (conversations.length > 0 || loading) return;
    fetchConversations();
  }, [fetchConversations]);

  return {
    conversations,
    activeId,
    loading,
    create: createConversation,
    remove: deleteConversation,
    togglePin,
    rename: renameConversation,
    setActive,
  };
}
