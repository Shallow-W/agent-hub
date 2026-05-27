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
  archiveConversationLocal: (id: string) => void;
  deleteConversation: (id: string) => Promise<void>;
  togglePin: (id: string) => Promise<void>;
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
  activeConversationId: localStorage.getItem('agenthub_active_conv'),
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
    try {
      const conv = await convApi.createConversation(type, title);
      set((state) => ({
        conversations: sortConversations([...state.conversations, conv]),
        activeConversationId: conv.id,
      }));
      return conv;
    } catch {
      const { message } = await import('antd');
      message.error('创建对话失败');
      throw new Error('创建对话失败');
    }
  },

  archiveConversationLocal: (id) => {
    set((state) => {
      const next = state.conversations.filter((c) => c.id !== id);
      const activeId =
        state.activeConversationId === id
          ? (next[0]?.id ?? null)
          : state.activeConversationId;
      return { conversations: next, activeConversationId: activeId };
    });
  },

  deleteConversation: async (id) => {
    try {
      await convApi.deleteConversation(id);
      set((state) => {
        const next = state.conversations.filter((c) => c.id !== id);
        const activeId =
          state.activeConversationId === id
            ? (next[0]?.id ?? null)
            : state.activeConversationId;
        return { conversations: next, activeConversationId: activeId };
      });
    } catch {
      const { message } = await import('antd');
      message.error('删除对话失败');
    }
  },

  togglePin: async (id) => {
    try {
      await convApi.togglePin(id);
      set((state) => ({
        conversations: sortConversations(
          state.conversations.map((c) =>
            c.id === id ? { ...c, pinned: !c.pinned } : c,
          ),
        ),
      }));
    } catch {
      const { message } = await import('antd');
      message.error('置顶操作失败');
    }
  },

  setActive: (id) => {
    if (id) localStorage.setItem('agenthub_active_conv', id);
    else localStorage.removeItem('agenthub_active_conv');
    set({ activeConversationId: id, memberPanelOpen: false });
  },

  setMemberPanelOpen: (open) => {
    set({ memberPanelOpen: open });
  },
}));
