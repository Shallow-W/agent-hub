import { create } from 'zustand';
import type { Conversation, ConversationType } from '@/types/conversation';
import * as convApi from '@/api/conversation';

interface ConversationState {
  conversations: Conversation[];
  activeConversationId: string | null;
  memberPanelOpen: boolean;
  loading: boolean;
  fetchConversations: () => Promise<void>;
  createConversation: (type: ConversationType, title: string) => Promise<Conversation>;
  deleteConversation: (id: string) => Promise<void>;
  togglePin: (id: string, pinned: boolean) => Promise<void>;
  setActive: (id: string | null) => void;
  setMemberPanelOpen: (open: boolean) => void;
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
  activeConversationId: null,
  memberPanelOpen: false,
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
      return { conversations: next, activeConversationId: activeId };
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

  setActive: (id) => {
    set({ activeConversationId: id, memberPanelOpen: false });
  },

  setMemberPanelOpen: (open) => {
    set({ memberPanelOpen: open });
  },
}));
