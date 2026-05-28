import { create } from 'zustand';
import type { Conversation, ConversationAgent, ConversationType } from '@/types/conversation';
import * as convApi from '@/api/conversation';

interface ConversationState {
  conversations: Conversation[];
  conversationAgents: Record<string, ConversationAgent[]>;
  activeConversationId: string | null;
  loading: boolean;
  fetchConversations: () => Promise<void>;
  createConversation: (type: ConversationType, title: string) => Promise<Conversation>;
  deleteConversation: (id: string) => Promise<void>;
  togglePin: (id: string, pinned: boolean) => Promise<void>;
  fetchConversationAgents: (id: string) => Promise<void>;
  addConversationAgent: (id: string, agentId: string) => Promise<ConversationAgent>;
  removeConversationAgent: (id: string, agentId: string) => Promise<void>;
  setActive: (id: string | null) => void;
}

/** 置顶优先，再按更新时间倒序 */
function sortConversations(list: Conversation[]): Conversation[] {
  return [...list].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
  });
}

export const useConversationStore = create<ConversationState>((set) => ({
  conversations: [],
  conversationAgents: {},
  activeConversationId: null,
  loading: false,

  fetchConversations: async () => {
    set({ loading: true });
    try {
      const list = await convApi.getConversations();
      set({ conversations: sortConversations(list) });
    } finally {
      set({ loading: false });
    }
  },

  createConversation: async (type, title) => {
    const conv = await convApi.createConversation(type, title);
    set((state) => ({
      conversations: sortConversations([...state.conversations, conv]),
      activeConversationId: conv.id,
    }));
    return conv;
  },

  deleteConversation: async (id) => {
    await convApi.deleteConversation(id);
    set((state) => {
      const next = state.conversations.filter((c) => c.id !== id);
      const activeId =
        state.activeConversationId === id
          ? (next[0]?.id ?? null)
          : state.activeConversationId;
      const nextAgents = { ...state.conversationAgents };
      delete nextAgents[id];
      return {
        conversations: next,
        activeConversationId: activeId,
        conversationAgents: nextAgents,
      };
    });
  },

  togglePin: async (id, pinned) => {
    await convApi.togglePin(id, pinned);
    set((state) => ({
      conversations: sortConversations(
        state.conversations.map((c) =>
          c.id === id ? { ...c, pinned } : c,
        ),
      ),
    }));
  },

  fetchConversationAgents: async (id) => {
    const agents = await convApi.getConversationAgents(id);
    set((state) => ({
      conversationAgents: {
        ...state.conversationAgents,
        [id]: agents,
      },
    }));
  },

  addConversationAgent: async (id, agentId) => {
    const item = await convApi.addConversationAgent(id, agentId);
    set((state) => {
      const current = state.conversationAgents[id] ?? [];
      const next = [
        ...current.filter((agent) => agent.agent_id !== item.agent_id),
        item,
      ];
      return {
        conversationAgents: {
          ...state.conversationAgents,
          [id]: next,
        },
      };
    });
    return item;
  },

  removeConversationAgent: async (id, agentId) => {
    await convApi.removeConversationAgent(id, agentId);
    set((state) => {
      const current = state.conversationAgents[id] ?? [];
      return {
        conversationAgents: {
          ...state.conversationAgents,
          [id]: current.filter((agent) => agent.agent_id !== agentId),
        },
      };
    });
  },

  setActive: (id) => {
    set({ activeConversationId: id });
  },
}));
