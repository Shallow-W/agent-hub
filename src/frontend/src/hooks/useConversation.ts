import { useEffect } from 'react';
import { useConversationStore } from '@/store/conversationStore';

export function useConversation() {
  const conversations = useConversationStore((s) => s.conversations);
  const conversationAgents = useConversationStore((s) => s.conversationAgents);
  const activeId = useConversationStore((s) => s.activeConversationId);
  const loading = useConversationStore((s) => s.loading);
  const fetchConversations = useConversationStore((s) => s.fetchConversations);
  const createConversation = useConversationStore((s) => s.createConversation);
  const deleteConversation = useConversationStore((s) => s.deleteConversation);
  const togglePin = useConversationStore((s) => s.togglePin);
  const fetchConversationAgents = useConversationStore((s) => s.fetchConversationAgents);
  const addConversationAgent = useConversationStore((s) => s.addConversationAgent);
  const removeConversationAgent = useConversationStore((s) => s.removeConversationAgent);
  const setActive = useConversationStore((s) => s.setActive);

  useEffect(() => {
    fetchConversations();
  }, [fetchConversations]);

  return {
    conversations,
    conversationAgents,
    activeId,
    loading,
    create: createConversation,
    remove: deleteConversation,
    togglePin,
    fetchConversationAgents,
    addConversationAgent,
    removeConversationAgent,
    setActive,
  };
}
