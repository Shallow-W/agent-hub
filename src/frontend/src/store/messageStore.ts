import { create } from 'zustand';
import { message as antdMessage } from '@/utils/message';
import type { Message, MessageBlock, OptimisticMessage, ReplyToPreview, AgentEvent, MessageStatus } from '@/types/message';
import type { AttachmentPayload } from '@/types/attachment';
import * as msgApi from '@/api/message';
import { PAGE_SIZE, MAX_MESSAGES, RECALL_DEDUP_TTL_MS } from '@/config/constants';
import { reduceEvents, initialStreamingState } from './streamingReducer';

interface MessageState {
  /** conversationId → 消息列表（含 streaming placeholder，单一数据源） */
  messages: Record<string, Message[]>;
  /** messageId → task_id（StopButton 取消时回传后端定位 daemon task） */
  streamingTaskIds: Record<string, string>;
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
  deleteMessage: (conversationId: string, messageId: string) => Promise<void>;
  toggleMessagePin: (conversationId: string, messageId: string, pinned: boolean) => Promise<void>;
  addMessage: (conversationId: string, message: Message) => void;
  /** 新路径：把 daemon 透传的 AgentEvent[] 按 kind 聚合累积到 messages 数组里的 placeholder。 */
  appendDeltas: (
    conversationId: string,
    messageId: string,
    deltas: AgentEvent[],
    meta?: { taskId?: string; agentId?: string; agentName?: string },
  ) => void;
  completeStreaming: (
    conversationId: string,
    messageId: string,
    fullMessage: Message,
  ) => void;
  /** StopButton 取消时：把 placeholder 状态切到 canceled，清理 streamingTaskIds。 */
  cancelStreaming: (
    conversationId: string,
    messageId: string,
  ) => void;
  retryOptimistic: (conversationId: string, tempId: string) => Promise<void>;
  removeOptimistic: (conversationId: string, tempId: string) => void;
  incrementUnread: (conversationId: string) => void;
  markAllRead: (conversationId: string) => void;
  isConversationRead: (conversationId: string) => boolean;
  handleRecallPush: (conversationId: string, messageId: string) => void;
}


/** Max messages kept per conversation to prevent unbounded memory growth */


let tempIdCounter = 0;
function generateTempId(): string {
  return `__temp_${Date.now()}_${++tempIdCounter}`;
}

/**
 * 解析服务端推送的 blocks_json 字符串。
 *
 * 服务端 model.Message 只有 BlocksJSON 字符串字段（没有 Blocks 数组），
 * task.complete 时 backend SplitTextBlocksByCardFences 已把 text block 里的
 * fenced JSON 切分为 card kind block，写入了 blocks_json。
 *
 * 容错：JSON.parse 失败、非数组、空数组都返回 null，调用方 fallback 到本地 blocks。
 * 返回 null 时调用方应继续 fallback 到 message.blocks 或 dup.blocks。
 */
function parseServerBlocksJSON(blocksJSON: string | undefined | null): MessageBlock[] | null {
  if (!blocksJSON || typeof blocksJSON !== 'string') return null;
  try {
    const parsed = JSON.parse(blocksJSON);
    if (!Array.isArray(parsed) || parsed.length === 0) return null;
    return parsed as MessageBlock[];
  } catch {
    return null;
  }
}

const recentlyRecalled = new Map<string, number>();

function isRecentlyRecalled(messageId: string): boolean {
  const ts = recentlyRecalled.get(messageId);
  if (!ts) return false;
  if (Date.now() - ts > RECALL_DEDUP_TTL_MS) {
    recentlyRecalled.delete(messageId);
    return false;
  }
  return true;
}

// Periodic cleanup of expired recall dedup entries
setInterval(() => {
  const now = Date.now();
  for (const [id, ts] of recentlyRecalled) {
    if (now - ts > RECALL_DEDUP_TTL_MS) {
      recentlyRecalled.delete(id);
    }
  }
}, 60_000);

export const useMessageStore = create<MessageState>((set, get) => ({
  messages: {},
  streamingTaskIds: {},
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
      recentlyRecalled.set(messageId, Date.now());
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

  deleteMessage: async (conversationId, messageId) => {
    try {
      await msgApi.hideMessage(conversationId, messageId);
      set((state) => {
        const list = (state.messages[conversationId] ?? []).filter((m) => m.id !== messageId);
        return { messages: { ...state.messages, [conversationId]: list } };
      });
    } catch (err) {
      console.error('delete message failed:', err);
      antdMessage.error('删除失败，请重试');
    }
  },

  toggleMessagePin: async (conversationId, messageId, pinned) => {
    const applyPinned = (value: boolean) => {
      set((state) => {
        const list = (state.messages[conversationId] ?? []).map((m) =>
          m.id === messageId ? { ...m, pinned: value } : m,
        );
        return { messages: { ...state.messages, [conversationId]: list } };
      });
    };

    applyPinned(!pinned);
    try {
      if (pinned) {
        await msgApi.unpinMessage(conversationId, messageId);
        antdMessage.success('已取消 Pin');
      } else {
        await msgApi.pinMessage(conversationId, messageId);
        antdMessage.success('已 Pin 到上下文黑板');
      }
    } catch (err) {
      console.error('toggle message pin failed:', err);
      applyPinned(pinned);
      antdMessage.error('Pin 操作失败，请重试');
    }
  },

  addMessage: (conversationId, message) => {
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      const dupIdx = existing.findIndex((m) => m.id === message.id);
      if (dupIdx !== -1) {
        const dup = existing[dupIdx]!;
        // 合并服务端推送的字段——卡片更新（UpdateMessageCardsAndBroadcast）
        // 会通过 message.complete 推送新的 cards_json/cards，必须覆盖本地。
        // reply_to_message 仅在本地缺失时补齐（避免覆盖已有数据）。
        // blocks 优先级（PR 修复）：服务端 blocks_json（权威，已切分为 card kind block）
        // > 服务端 blocks 数组 > 本地 placeholder 累积的 blocks（流式原文）。
        // 之前 bug：后端 model.Message 只有 BlocksJSON 字符串，WS 推送时 blocks 字段
        // 永远为 undefined，导致本地 placeholder 的原始 text block 胜出，渲染为 fenced JSON 原文。
        const serverBlocksFromJSON = parseServerBlocksJSON(message.blocks_json);
        const mergedBlocks = serverBlocksFromJSON && serverBlocksFromJSON.length > 0
          ? serverBlocksFromJSON
          : (message.blocks ?? dup.blocks);
        const merged: Message = {
          ...dup,
          ...message,
          ...(message.cards_json !== undefined && message.cards_json !== null
            ? { cards_json: message.cards_json, cards: message.cards ?? dup.cards }
            : {}),
          ...(message.artifacts_json !== undefined && message.artifacts_json !== null
            ? { artifacts_json: message.artifacts_json }
            : {}),
          ...(message.artifacts !== undefined ? { artifacts: message.artifacts } : {}),
          ...(message.attachments !== undefined ? { attachments: message.attachments } : {}),
          ...(message.reply_to_message && !dup.reply_to_message
            ? { reply_to_message: message.reply_to_message }
            : {}),
          blocks: mergedBlocks,
        };
        const next = [...existing];
        next[dupIdx] = merged;
        // 当服务端推送终态 status（complete/error/canceled）时，清理 streamingTaskIds。
        // message.complete 全量推送走 addMessage 而非 completeStreaming，若不在此清理，
        // 占位符虽切到 complete，streamingTaskIds[messageId] 残留 → StopButton 不卸载。
        const nextTaskIds = (message.status && message.status !== 'streaming' && state.streamingTaskIds[message.id])
          ? (() => {
              const tids = { ...state.streamingTaskIds };
              delete tids[message.id];
              return tids;
            })()
          : state.streamingTaskIds;
        return {
          messages: { ...state.messages, [conversationId]: next },
          ...(nextTaskIds !== state.streamingTaskIds ? { streamingTaskIds: nextTaskIds } : {}),
        };
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

  appendDeltas: (conversationId, messageId, deltas, meta) => {
    if (!deltas || deltas.length === 0) return;
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      const placeholderIdx = existing.findIndex((m) => m.id === messageId);

      // 用 prev blocks 作为 reducer 起点（流式期间持续累积）。
      // PR1 后统一走 reduceEvents，消除 inline switch-case 的 dual-implementation drift。
      const prevBlocks: MessageBlock[] = placeholderIdx >= 0
        ? (existing[placeholderIdx]!.blocks ?? [])
        : [];
      const reduced = reduceEvents(deltas, {
        ...initialStreamingState,
        blocks: prevBlocks,
        taskId: meta?.taskId,
        agentId: meta?.agentId,
      });
      const next = reduced.blocks;

      // 创建或更新 placeholder——首次见到 delta 时创建，后续更新 blocks。
      // PR3：从 WS payload 的 meta.agentName 取真实 username，避免 fallback 到"助手"。
      // 双保险：meta.agentName 来自后端 daemonHub.taskAgents 反查；同时 artifacts_json
      // 在预创建时已写入相同 agent_name（backend createAgentReply）。
      let nextMessages: Record<string, Message[]>;
      let nextTaskIds = state.streamingTaskIds;

      const agentArtifacts = meta?.agentName || meta?.agentId
        ? JSON.stringify({
            ...(meta?.agentId ? { agent_id: meta.agentId } : {}),
            ...(meta?.agentName ? { agent_name: meta.agentName } : {}),
          })
        : undefined;

      if (placeholderIdx < 0) {
        const placeholder: Message = {
          id: messageId,
          conversation_id: conversationId,
          role: 'assistant',
          content: '',
          artifacts_json: agentArtifacts ?? null,
          created_at: new Date().toISOString(),
          status: 'streaming' as MessageStatus,
          blocks: next,
          ...(meta?.taskId ? { task_id: meta.taskId } : {}),
          ...(meta?.agentName ? { username: meta.agentName } : {}),
        };
        nextMessages = {
          ...state.messages,
          [conversationId]: [...existing, placeholder],
        };
        if (meta?.taskId) {
          nextTaskIds = { ...state.streamingTaskIds, [messageId]: meta.taskId };
        }
      } else {
        nextMessages = {
          ...state.messages,
          [conversationId]: existing.map((m, i) => {
            if (i !== placeholderIdx) return m;
            // 合并 meta 字段：保留已有 username/artifacts_json 除非 meta 有更新。
            // 首次创建时已写入，后续 delta 批次一般不携带 meta——不覆盖。
            return {
              ...m,
              blocks: next,
              ...(m.task_id === undefined && meta?.taskId ? { task_id: meta.taskId } : {}),
              // 不覆盖 username——首次设置后保留（防止后续批次 meta 为空时把 username 清掉）。
            };
          }),
        };
      }

      return {
        messages: nextMessages,
        streamingTaskIds: nextTaskIds,
      };
    });
  },

  completeStreaming: (conversationId, messageId, fullMessage) => {
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      // 找到 placeholder（之前 appendDeltas 创建），与服务端 fullMessage 合并。
      // PR3：保留本地 placeholder 累积的 blocks（除非服务端推送权威 blocks/blocks_json）；
      // status 切到 fullMessage.status ?? 'complete'；username 优先保留本地（防 fallback）。
      const placeholder = existing.find((m) => m.id === messageId);
      const placeholderBlocks = placeholder?.blocks;
      const placeholderUsername = placeholder?.username;

      // blocks 优先级（PR 修复）：服务端 blocks_json（权威，已切分为 card kind block）
      // > 服务端 blocks 数组 > 本地 placeholder 累积的 blocks。
      const serverBlocksFromJSON = parseServerBlocksJSON(fullMessage.blocks_json);
      const mergedBlocks = serverBlocksFromJSON && serverBlocksFromJSON.length > 0
        ? serverBlocksFromJSON
        : (fullMessage.blocks ?? placeholderBlocks);

      const mergedMessage: Message = {
        ...fullMessage,
        blocks: mergedBlocks,
        status: (fullMessage.status ?? 'complete') as MessageStatus,
        // username 保留 placeholder 中的（完整名字），防止服务端广播未带 username 时 fallback。
        ...(placeholderUsername && !fullMessage.username ? { username: placeholderUsername } : {}),
      };

      let nextMessages: Record<string, Message[]>;
      if (existing.some((m) => m.id === fullMessage.id)) {
        nextMessages = {
          ...state.messages,
          [conversationId]: existing.map((m) =>
            m.id === fullMessage.id ? { ...m, ...mergedMessage } : m,
          ),
        };
      } else {
        const appended = [...existing, mergedMessage];
        const trimmed = appended.length > MAX_MESSAGES ? appended.slice(appended.length - MAX_MESSAGES) : appended;
        nextMessages = {
          ...state.messages,
          [conversationId]: trimmed,
        };
      }

      // 清理 streamingTaskIds
      const nextTaskIds = { ...state.streamingTaskIds };
      delete nextTaskIds[messageId];

      return {
        messages: nextMessages,
        streamingTaskIds: nextTaskIds,
      };
    });
  },

  cancelStreaming: (conversationId, messageId) => {
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      const updated = existing.map((m) =>
        m.id === messageId ? { ...m, status: 'canceled' as MessageStatus } : m
      );
      const nextTaskIds = { ...state.streamingTaskIds };
      delete nextTaskIds[messageId];
      return {
        messages: { ...state.messages, [conversationId]: updated },
        streamingTaskIds: nextTaskIds,
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
    if (isRecentlyRecalled(messageId)) return;
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

export function resetMessageStore() {
  useMessageStore.setState({
    messages: {},
    streamingTaskIds: {},
    hasMore: {},
    loading: {},
    optimisticMessages: {},
    unreadCounts: {},
    readConversations: {},
  });
}
