import { create } from 'zustand';
import type { Conversation, ConversationType } from '@/types/conversation';
import * as convApi from '@/api/conversation';

const DIRECT_AGENT_CHATS_KEY = 'agenthub_direct_agent_chats';

interface ConversationState {
  conversations: Conversation[];
  activeConversationId: string | null;
  directAgentChats: Record<string, string>;
  memberPanelOpen: boolean;
  loading: boolean;
  _fetching: boolean;
  fetchConversations: () => Promise<void>;
  createConversation: (type: ConversationType, title: string) => Promise<Conversation>;
  archiveConversationLocal: (id: string) => void;
  deleteConversation: (id: string) => Promise<void>;
  togglePin: (id: string) => Promise<void>;
  renameConversation: (id: string, title: string) => Promise<void>;
  setActive: (id: string | null) => void;
  bindDirectAgentChat: (conversationId: string, agentId: string) => void;
  setMemberPanelOpen: (open: boolean) => void;
}

function loadDirectAgentChats(): Record<string, string> {
  try {
    const raw = localStorage.getItem(DIRECT_AGENT_CHATS_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed as Record<string, string>
      : {};
  } catch {
    return {};
  }
}

/** 置顶优先，再按更新时间倒序 */
function sortConversations(list: Conversation[]): Conversation[] {
  return [...list].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
  });
}

export const useConversationStore = create<ConversationState>((set, get) => ({
  conversations: [],
  activeConversationId: localStorage.getItem('agenthub_active_conv'),
  directAgentChats: loadDirectAgentChats(),
  memberPanelOpen: false,
  loading: false,
  _fetching: false,

  fetchConversations: async () => {
    if (get()._fetching) return;
    set({ _fetching: true, loading: true });
    try {
      const list = await convApi.getConversations();
      set({ conversations: sortConversations(list) });
    } finally {
      set({ loading: false, _fetching: false });
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

  renameConversation: async (id, title) => {
    const prev = get().conversations;
    set((state) => ({
      conversations: sortConversations(
        state.conversations.map((c) =>
          c.id === id ? { ...c, title } : c,
        ),
      ),
    }));
    try {
      await convApi.renameConversation(id, title);
    } catch {
      set({ conversations: prev });
      const { message } = await import('antd');
      message.error('重命名失败');
    }
  },

  setActive: (id) => {
    if (id) localStorage.setItem('agenthub_active_conv', id);
    else localStorage.removeItem('agenthub_active_conv');
    set({ activeConversationId: id, memberPanelOpen: false });
  },

  bindDirectAgentChat: (conversationId, agentId) => {
    set((state) => {
      const next = { ...state.directAgentChats, [conversationId]: agentId };
      localStorage.setItem(DIRECT_AGENT_CHATS_KEY, JSON.stringify(next));
      return { directAgentChats: next };
    });
  },

  setMemberPanelOpen: (open) => {
    set({ memberPanelOpen: open });
  },
}));
