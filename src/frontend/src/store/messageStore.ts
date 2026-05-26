import { create } from 'zustand';
import type { Message } from '@/types/message';
import type { OptimisticMessage } from '@/types/message';
import * as msgApi from '@/api/message';

interface MessageState {
  /** conversationId → 消息列表 */
  messages: Record<string, Message[]>;
  /** conversationId → 流式拼接的临时内容 */
  streamingContent: Record<string, string>;
  /** conversationId → 是否还有更早的消息可加载 */
  hasMore: Record<string, boolean>;
  loading: boolean;
  /** conversationId → optimistic messages (pending send) */
  optimisticMessages: Record<string, OptimisticMessage[]>;
  /** conversationId → unread count */
  unreadCounts: Record<string, number>;

  fetchMessages: (conversationId: string, before?: string) => Promise<void>;
  sendMessage: (conversationId: string, content: string) => Promise<void>;
  addMessage: (conversationId: string, message: Message) => void;
  updateStreaming: (
    conversationId: string,
    messageId: string,
    content: string,
  ) => void;
  completeStreaming: (
    conversationId: string,
    messageId: string,
    fullMessage: Message,
  ) => void;
  retryOptimistic: (conversationId: string, tempId: string) => Promise<void>;
  removeOptimistic: (conversationId: string, tempId: string) => void;
  incrementUnread: (conversationId: string) => void;
  markAllRead: (conversationId: string) => void;
}

const PAGE_SIZE = 50;

let tempIdCounter = 0;
function generateTempId(): string {
  return `__temp_${Date.now()}_${++tempIdCounter}`;
}

export const useMessageStore = create<MessageState>((set, get) => ({
  messages: {},
  streamingContent: {},
  hasMore: {},
  loading: false,
  optimisticMessages: {},
  unreadCounts: {},

  fetchMessages: async (conversationId, before) => {
    set({ loading: true });
    try {
      const list = await msgApi.getMessages(
        conversationId,
        before,
        PAGE_SIZE,
      );
      set((state) => {
        const existing = state.messages[conversationId] ?? [];
        // 历史消息拼在前面（按时间升序排列）
        const merged = [...list, ...existing];
        return {
          messages: { ...state.messages, [conversationId]: merged },
          hasMore: {
            ...state.hasMore,
            [conversationId]: list.length >= PAGE_SIZE,
          },
        };
      });
    } finally {
      set({ loading: false });
    }
  },

  sendMessage: async (conversationId, content) => {
    const tempId = generateTempId();
    const optimistic: OptimisticMessage = {
      id: tempId,
      conversation_id: conversationId,
      role: 'user',
      content,
      artifacts_json: null,
      created_at: new Date().toISOString(),
      optimistic: true,
      optimisticStatus: 'sending',
    };

    // Add optimistic message immediately
    set((state) => {
      const existing = state.optimisticMessages[conversationId] ?? [];
      return {
        optimisticMessages: {
          ...state.optimisticMessages,
          [conversationId]: [...existing, optimistic],
        },
      };
    });

    try {
      const msg = await msgApi.sendMessage(conversationId, content, 'user');
      get().addMessage(conversationId, msg);
      // Remove optimistic message on success
      set((state) => {
        const remaining = (state.optimisticMessages[conversationId] ?? [])
          .filter((m) => m.id !== tempId);
        return {
          optimisticMessages: {
            ...state.optimisticMessages,
            [conversationId]: remaining,
          },
        };
      });
    } catch {
      // Mark optimistic message as failed
      set((state) => {
        const updated = (state.optimisticMessages[conversationId] ?? []).map(
          (m) => m.id === tempId ? { ...m, optimisticStatus: 'failed' as const } : m,
        );
        return {
          optimisticMessages: {
            ...state.optimisticMessages,
            [conversationId]: updated,
          },
        };
      });
    }
  },

  addMessage: (conversationId, message) => {
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      // 按 ID 去重，防止乐观消息与服务端推送重复
      if (existing.some((m) => m.id === message.id)) {
        return state;
      }
      return {
        messages: {
          ...state.messages,
          [conversationId]: [...existing, message],
        },
      };
    });
  },

  updateStreaming: (conversationId, _messageId, content) => {
    set((state) => {
      const prev = state.streamingContent[conversationId] ?? '';
      return {
        streamingContent: {
          ...state.streamingContent,
          [conversationId]: prev + content,
        },
      };
    });
  },

  completeStreaming: (conversationId, _messageId, fullMessage) => {
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      // 按 ID 去重
      if (existing.some((m) => m.id === fullMessage.id)) {
        const next = { ...state.streamingContent };
        delete next[conversationId];
        return { streamingContent: next };
      }
      const next = { ...state.streamingContent };
      delete next[conversationId];
      return {
        messages: {
          ...state.messages,
          [conversationId]: [...existing, fullMessage],
        },
        streamingContent: next,
      };
    });
  },

  retryOptimistic: async (conversationId, tempId) => {
    const state = get();
    const optMsg = (state.optimisticMessages[conversationId] ?? [])
      .find((m) => m.id === tempId);
    if (!optMsg) return;

    // Mark as sending again
    set((s) => {
      const updated = (s.optimisticMessages[conversationId] ?? []).map(
        (m) => m.id === tempId ? { ...m, optimisticStatus: 'sending' as const } : m,
      );
      return {
        optimisticMessages: {
          ...s.optimisticMessages,
          [conversationId]: updated,
        },
      };
    });

    try {
      const msg = await msgApi.sendMessage(conversationId, optMsg.content, 'user');
      get().addMessage(conversationId, msg);
      // Remove on success
      set((s) => {
        const remaining = (s.optimisticMessages[conversationId] ?? [])
          .filter((m) => m.id !== tempId);
        return {
          optimisticMessages: {
            ...s.optimisticMessages,
            [conversationId]: remaining,
          },
        };
      });
    } catch {
      // Mark as failed again
      set((s) => {
        const updated = (s.optimisticMessages[conversationId] ?? []).map(
          (m) => m.id === tempId ? { ...m, optimisticStatus: 'failed' as const } : m,
        );
        return {
          optimisticMessages: {
            ...s.optimisticMessages,
            [conversationId]: updated,
          },
        };
      });
    }
  },

  removeOptimistic: (conversationId, tempId) => {
    set((state) => {
      const remaining = (state.optimisticMessages[conversationId] ?? [])
        .filter((m) => m.id !== tempId);
      return {
        optimisticMessages: {
          ...state.optimisticMessages,
          [conversationId]: remaining,
        },
      };
    });
  },

  incrementUnread: (conversationId) => {
    set((state) => {
      const current = state.unreadCounts[conversationId] ?? 0;
      return {
        unreadCounts: {
          ...state.unreadCounts,
          [conversationId]: current + 1,
        },
      };
    });
  },

  markAllRead: (conversationId) => {
    set((state) => ({
      unreadCounts: {
        ...state.unreadCounts,
        [conversationId]: 0,
      },
    }));
  },
}));
