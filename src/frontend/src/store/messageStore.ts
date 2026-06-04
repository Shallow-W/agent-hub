import { create } from 'zustand';
import { message as antdMessage } from 'antd';
import type { Message, OptimisticMessage, ReplyToPreview } from '@/types/message';
import type { AttachmentPayload } from '@/types/attachment';
import * as msgApi from '@/api/message';

interface MessageState {
  /** conversationId → 消息列表 */
  messages: Record<string, Message[]>;
  /** conversationId → 流式拼接的临时内容 */
  streamingContent: Record<string, string>;
  /** conversationId → 是否还有更早的消息可加载 */
  hasMore: Record<string, boolean>;
  /** conversationId → 是否正在加载 */
  loading: Record<string, boolean>;
  /** conversationId → optimistic messages (pending send) */
  optimisticMessages: Record<string, OptimisticMessage[]>;
  /** conversationId → unread count */
  unreadCounts: Record<string, number>;
  /** Track which conversations the current user considers "read" */
  readConversations: Record<string, boolean>;

  fetchMessages: (conversationId: string, before?: string) => Promise<void>;
  sendMessage: (
    conversationId: string,
    content: string,
    attachments?: AttachmentPayload[],
    replyTo?: string,
    replyPreview?: ReplyToPreview,
    mentions?: string[],
    agentId?: string,
  ) => Promise<void>;
  recall: (conversationId: string, messageId: string) => Promise<void>;
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
  isConversationRead: (conversationId: string) => boolean;
  handleRecallPush: (conversationId: string, messageId: string) => void;
}

const PAGE_SIZE = 200;
/** Max messages kept per conversation to prevent unbounded memory growth */
const MAX_MESSAGES = 200;

let tempIdCounter = 0;
function generateTempId(): string {
  return `__temp_${Date.now()}_${++tempIdCounter}`;
}

const recentlyRecalled = new Set<string>();
const RECALL_DEDUP_TTL = 30_000;

export const useMessageStore = create<MessageState>((set, get) => ({
  messages: {},
  streamingContent: {},
  hasMore: {},
  loading: {},
  optimisticMessages: {},
  unreadCounts: {},
  readConversations: {},

  fetchMessages: async (conversationId, before) => {
    set((s) => ({ loading: { ...s.loading, [conversationId]: true } }));
    try {
      const list = await msgApi.getMessages(
        conversationId,
        before,
        PAGE_SIZE,
      );
      set((state) => {
        // before 有值表示翻页加载更多，拼在前面；否则是首次加载，覆盖旧数据
        const existing = before ? (state.messages[conversationId] ?? []) : [];
        // 后端返回 DESC，需要按 created_at ASC 排序保证旧消息在前
        const merged = [...list, ...existing].sort(
          (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
        );
        // 只在首次加载时裁剪；加载更多时保留历史消息
        const trimmed = (!before && merged.length > MAX_MESSAGES) ? merged.slice(merged.length - MAX_MESSAGES) : merged;
        return {
          messages: { ...state.messages, [conversationId]: trimmed },
          hasMore: {
            ...state.hasMore,
            [conversationId]: list.length >= PAGE_SIZE,
          },
          loading: { ...state.loading, [conversationId]: false },
        };
      });
    } catch {
      set((s) => ({ loading: { ...s.loading, [conversationId]: false } }));
    }
  },

  sendMessage: async (conversationId, content, attachments?, replyTo?, replyPreview?, mentions?, agentId?) => {
    const resolveReplyPreview = (replyToId?: string): ReplyToPreview | null => {
      if (!replyToId) return null;
      const existing = get().messages[conversationId] ?? [];
      const target = existing.find((m) => m.id === replyToId);
      if (!target) return null;
      return {
        id: target.id,
        content: target.content ?? '',
        sender_id: target.sender_id,
        username: target.username,
        deleted_at: null,
      };
    };
    const resolvedReplyPreview = replyPreview ?? resolveReplyPreview(replyTo ?? undefined);
    const tempId = generateTempId();
    const optimistic: OptimisticMessage = {
      id: tempId,
      conversation_id: conversationId,
      role: 'user',
      content,
      artifacts_json: null,
      created_at: new Date().toISOString(),
      reply_to: replyTo ?? null,
      reply_to_message: resolvedReplyPreview ?? undefined,
      optimistic: true,
      optimisticStatus: 'sending',
      pendingAttachments: attachments,
      pendingAgentId: agentId,
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
      const result = await msgApi.sendMessage(conversationId, content, 'user', attachments, replyTo, mentions, agentId);
      const msg = result.user_message;
      if (!msg) throw new Error('Server returned empty user_message');
      const patchedMsg = resolvedReplyPreview && !msg.reply_to_message
        ? {
            ...msg,
            reply_to: msg.reply_to ?? replyTo ?? null,
            reply_to_message: resolvedReplyPreview,
          }
        : msg;
      get().addMessage(conversationId, patchedMsg);
      if (result.agent_message) {
        get().addMessage(conversationId, result.agent_message);
      }
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
    } catch (err) {
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
      throw err;
    }
  },

  recall: async (conversationId, messageId) => {
    try {
      recentlyRecalled.add(messageId);
      setTimeout(() => recentlyRecalled.delete(messageId), RECALL_DEDUP_TTL);
      await msgApi.recallMessage(conversationId, messageId);
      set((state) => {
        const list = (state.messages[conversationId] ?? []).map((m) =>
          m.id === messageId
            ? { ...m, content: '你撤回了一条消息', role: 'system' as const, attachments: undefined, reply_to: null, reply_to_message: null }
            : m
        );
        return { messages: { ...state.messages, [conversationId]: list } };
      });
    } catch (err) {
      console.error('recall failed:', err);
      antdMessage.error('撤回失败，请重试');
    }
  },

  addMessage: (conversationId, message) => {
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      const dupIdx = existing.findIndex((m) => m.id === message.id);
      if (dupIdx !== -1) {
        const dup = existing[dupIdx]!;
        if (message.reply_to_message && !dup.reply_to_message) {
          const merged = [...existing];
          merged[dupIdx] = { ...dup, reply_to_message: message.reply_to_message } as Message;
          return { messages: { ...state.messages, [conversationId]: merged } };
        }
        return state;
      }
      const next = [...existing, message];
      // Trim oldest messages from the beginning if exceeding cap
      const trimmed = next.length > MAX_MESSAGES ? next.slice(next.length - MAX_MESSAGES) : next;
      return {
        messages: {
          ...state.messages,
          [conversationId]: trimmed,
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
      const appended = [...existing, fullMessage];
      const trimmed = appended.length > MAX_MESSAGES ? appended.slice(appended.length - MAX_MESSAGES) : appended;
      return {
        messages: {
          ...state.messages,
          [conversationId]: trimmed,
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
      const result = await msgApi.sendMessage(
        conversationId,
        optMsg.content,
        'user',
        optMsg.pendingAttachments,
        optMsg.reply_to ?? undefined,
        optMsg.mentions,
        optMsg.pendingAgentId,
      );
      if (!result.user_message) throw new Error('Server returned empty user_message');
      get().addMessage(conversationId, result.user_message);
      if (result.agent_message) {
        get().addMessage(conversationId, result.agent_message);
      }
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
    } catch (err) {
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
      throw err;
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
    set((state) => {
      if (state.readConversations[conversationId] && state.unreadCounts[conversationId] === 0) {
        return state; // 已经是已读状态，跳过更新
      }
      return {
        unreadCounts: {
          ...state.unreadCounts,
          [conversationId]: 0,
        },
        readConversations: {
          ...state.readConversations,
          [conversationId]: true,
        },
      };
    });
  },

  isConversationRead: (conversationId) => {
    return !!get().readConversations[conversationId];
  },

  handleRecallPush: (conversationId, messageId) => {
    if (recentlyRecalled.has(messageId)) return;
    set((state) => {
      const list = (state.messages[conversationId] ?? []).map((m) =>
        m.id === messageId
          ? { ...m, content: '一条消息被撤回', role: 'system' as const, attachments: undefined, reply_to: null, reply_to_message: null }
          : m
      );
      return { messages: { ...state.messages, [conversationId]: list } };
    });
  },
}));
