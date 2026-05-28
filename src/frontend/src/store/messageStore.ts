import { create } from 'zustand';
import type { Message } from '@/types/message';
import * as msgApi from '@/api/message';

interface MessageState {
  /** conversationId → 消息列表 */
  messages: Record<string, Message[]>;
  /** conversationId → 流式拼接的临时内容 */
  streamingContent: Record<string, string>;
  /** conversationId → 是否还有更早的消息可加载 */
  hasMore: Record<string, boolean>;
  loading: boolean;

  fetchMessages: (conversationId: string, before?: string) => Promise<void>;
  sendMessage: (conversationId: string, content: string, agentId?: string) => Promise<void>;
  addMessage: (conversationId: string, message: Message) => void;
  replaceMessage: (
    conversationId: string,
    localId: string,
    message: Message,
  ) => void;
  removeMessage: (conversationId: string, messageId: string) => void;
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
}

const PAGE_SIZE = 50;

export const useMessageStore = create<MessageState>((set, get) => ({
  messages: {},
  streamingContent: {},
  hasMore: {},
  loading: false,

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

  sendMessage: async (conversationId, content, agentId) => {
    const localID = `local-${Date.now()}`;
    const localMessage: Message = {
      id: localID,
      conversation_id: conversationId,
      role: 'user',
      content,
      artifacts_json: null,
      created_at: new Date().toISOString(),
    };
    get().addMessage(conversationId, localMessage);
    if (agentId) {
      set((state) => ({
        streamingContent: {
          ...state.streamingContent,
          [conversationId]: 'Agent 正在思考...',
        },
      }));
    }
    try {
      const result = await msgApi.sendMessage(conversationId, content, 'user', agentId);
      get().replaceMessage(conversationId, localID, result.user_message);
      if (result.agent_message) {
        get().addMessage(conversationId, result.agent_message);
      }
    } catch (error) {
      get().removeMessage(conversationId, localID);
      throw error;
    } finally {
      set((state) => {
        const next = { ...state.streamingContent };
        delete next[conversationId];
        return { streamingContent: next };
      });
    }
  },

  addMessage: (conversationId, message) => {
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      return {
        messages: {
          ...state.messages,
          [conversationId]: [...existing, message],
        },
      };
    });
  },

  replaceMessage: (conversationId, localId, message) => {
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      return {
        messages: {
          ...state.messages,
          [conversationId]: existing.map((item) => (
            item.id === localId ? message : item
          )),
        },
      };
    });
  },

  removeMessage: (conversationId, messageId) => {
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      return {
        messages: {
          ...state.messages,
          [conversationId]: existing.filter((item) => item.id !== messageId),
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
}));
