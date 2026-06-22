import { create } from 'zustand';
import { message as antdMessage } from '@/utils/message';
import type { Message, MessageBlock, OptimisticMessage, ReplyToPreview, AgentEvent } from '@/types/message';
import type { AttachmentPayload } from '@/types/attachment';
import * as msgApi from '@/api/message';
import { PAGE_SIZE, MAX_MESSAGES, RECALL_DEDUP_TTL_MS } from '@/config/constants';

interface MessageState {
  /** conversationId → 消息列表 */
  messages: Record<string, Message[]>;
  /** conversationId → 流式拼接的临时内容（旧路径，仅字符串覆盖场景使用） */
  streamingContent: Record<string, string>;
  /** messageId → 流式累积的 block 列表（新路径，支持 text/thinking/tool_use/tool_result/error） */
  streamingBlocks: Record<string, MessageBlock[]>;
  /** messageId → 流式占位消息（携带 task_id / status='streaming'，供 MessageList 渲染） */
  streamingMessages: Record<string, Message>;
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
  updateStreaming: (
    conversationId: string,
    messageId: string,
    content: string,
  ) => void;
  /** 新路径：把 daemon 透传的 AgentEvent[] 按 kind 聚合累积到 streamingBlocks。 */
  appendDeltas: (
    conversationId: string,
    messageId: string,
    deltas: AgentEvent[],
    meta?: { taskId?: string; agentId?: string },
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


/** Max messages kept per conversation to prevent unbounded memory growth */


let tempIdCounter = 0;
function generateTempId(): string {
  return `__temp_${Date.now()}_${++tempIdCounter}`;
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
  streamingContent: {},
  streamingBlocks: {},
  streamingMessages: {},
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
        const merged: Message = {
          ...dup,
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
        };
        const next = [...existing];
        next[dupIdx] = merged;
        return { messages: { ...state.messages, [conversationId]: next } };
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

  appendDeltas: (conversationId, messageId, deltas, meta) => {
    if (!deltas || deltas.length === 0) return;
    set((state) => {
      const prevBlocks = state.streamingBlocks[messageId] ?? [];
      const next = [...prevBlocks];
      // 用于判断最近一个 block 是否可累积：必须是相同 kind 且非 tool_result/error
      // （这两种 block 一次性产出，不再累积）。
      const appendableKind = (kind: string): boolean =>
        kind === 'text' || kind === 'thinking' || kind === 'tool_use';

      for (const ev of deltas) {
        if (!ev) continue;
        const last = next[next.length - 1];
        switch (ev.type) {
          case 'text': {
            const text = ev.content ?? '';
            if (!text) break;
            if (last && last.kind === 'text') {
              next[next.length - 1] = { ...last, text: last.text + text };
            } else {
              next.push({ index: next.length, kind: 'text', text });
            }
            break;
          }
          case 'thinking': {
            const text = ev.content ?? '';
            if (!text) break;
            if (last && last.kind === 'thinking') {
              next[next.length - 1] = { ...last, text: last.text + text };
            } else {
              next.push({ index: next.length, kind: 'thinking', text });
            }
            break;
          }
          case 'tool_use': {
            const toolName = ev.tool ?? '';
            if (toolName) {
              // 工具名非空 → 开启一个新 tool_use block（输入参数后续 delta 会追加）
              next.push({
                index: next.length,
                kind: 'tool_use',
                text: '',
                tool_name: toolName,
                tool_use_id: ev.tool_use_id,
              });
            } else if (last && last.kind === 'tool_use') {
              // 空工具名 → input_json_delta，追加到当前 tool_use block
              const inputDelta = ev.content ?? '';
              if (inputDelta) {
                next[next.length - 1] = { ...last, text: last.text + inputDelta };
              }
            }
            break;
          }
          case 'tool_result': {
            // tool_result 始终是新 block（同一工具调用的结果与输入分开展示）
            next.push({
              index: next.length,
              kind: 'tool_result',
              text: ev.output ?? '',
              is_error: ev.isError === true,
            });
            break;
          }
          case 'error': {
            next.push({
              index: next.length,
              kind: 'error',
              text: ev.message ?? '生成失败',
              is_error: true,
            });
            break;
          }
          case 'turn_end':
            // turn_end 触发 completeStreaming，由调用方处理。
            break;
          default:
            // 未知 kind 忽略，不破坏流。
            break;
        }
        // 触发 React 状态更新——每次 append 都返回新数组引用，React.memo 的 block 组件
        // 仅在自身 block 引用变化时 re-render，相邻 block 不受影响。
        // （此处用 next 局部变量，最终在 set 一次性提交。）
        void appendableKind;
      }

      // 占位 streaming message：首次见到 delta 时创建，后续保留。
      // MessageList 渲染时把它当作额外的 assistant message（status=streaming），
      // 让 StopButton 能取到 task_id 并发 HTTP 取消。
      const prevPlaceholder = state.streamingMessages[messageId];
      let placeholder: Message;
      if (prevPlaceholder) {
        placeholder = prevPlaceholder;
      } else {
        placeholder = {
          id: messageId,
          conversation_id: conversationId,
          role: 'assistant',
          content: '',
          artifacts_json: null,
          created_at: new Date().toISOString(),
          status: 'streaming',
          ...(meta?.taskId ? { task_id: meta.taskId } : {}),
        };
      }

      return {
        streamingBlocks: {
          ...state.streamingBlocks,
          [messageId]: next,
        },
        streamingMessages: {
          ...state.streamingMessages,
          [messageId]: placeholder,
        },
      };
    });
  },

  completeStreaming: (conversationId, messageId, fullMessage) => {
    set((state) => {
      const existing = state.messages[conversationId] ?? [];
      // 流式累积的 block 落到 message.blocks，刷新页面前的最后一帧快照。
      // 服务端推送的 fullMessage 若已包含 blocks/blocks_json 则优先用服务端权威副本。
      const streamedBlocks = state.streamingBlocks[messageId];
      const mergedMessage: Message =
        streamedBlocks && streamedBlocks.length > 0 && !fullMessage.blocks && !fullMessage.blocks_json
          ? { ...fullMessage, blocks: streamedBlocks, status: fullMessage.status ?? 'complete' }
          : { ...fullMessage, status: fullMessage.status ?? 'complete' };
      // 按 ID 去重
      if (existing.some((m) => m.id === fullMessage.id)) {
        const merged = existing.map((m) =>
          m.id === fullMessage.id ? { ...m, ...mergedMessage } : m,
        );
        const nextStreamingBlocks = { ...state.streamingBlocks };
        delete nextStreamingBlocks[messageId];
        const nextStreamingMessages = { ...state.streamingMessages };
        delete nextStreamingMessages[messageId];
        const nextStreamingContent = { ...state.streamingContent };
        delete nextStreamingContent[conversationId];
        return {
          messages: { ...state.messages, [conversationId]: merged },
          streamingBlocks: nextStreamingBlocks,
          streamingMessages: nextStreamingMessages,
          streamingContent: nextStreamingContent,
        };
      }
      const nextStreamingBlocks = { ...state.streamingBlocks };
      delete nextStreamingBlocks[messageId];
      const nextStreamingMessages = { ...state.streamingMessages };
      delete nextStreamingMessages[messageId];
      const nextStreamingContent = { ...state.streamingContent };
      delete nextStreamingContent[conversationId];
      const appended = [...existing, mergedMessage];
      const trimmed = appended.length > MAX_MESSAGES ? appended.slice(appended.length - MAX_MESSAGES) : appended;
      return {
        messages: {
          ...state.messages,
          [conversationId]: trimmed,
        },
        streamingBlocks: nextStreamingBlocks,
        streamingMessages: nextStreamingMessages,
        streamingContent: nextStreamingContent,
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
    streamingContent: {},
    streamingBlocks: {},
    streamingMessages: {},
    hasMore: {},
    loading: {},
    optimisticMessages: {},
    unreadCounts: {},
    readConversations: {},
  });
}
