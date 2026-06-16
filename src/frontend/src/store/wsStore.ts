import { create } from 'zustand';
import { WebSocketClient, type WsStatus } from '@/api/websocket';

const agentTypingTimers = new Map<string, ReturnType<typeof setTimeout>>();

// ---------------------------------------------------------------------------
// 泛化 WS 事件发布/订阅
// 替代之前为 task.changed / conversation.role_changed 手写的独立 pubsub。
// 新增事件类型只需 onWsEvent('type', handler) 订阅。
// ---------------------------------------------------------------------------

type WsEventHandler = (data: unknown) => void;
const eventListeners = new Map<string, Set<WsEventHandler>>();

/** 订阅一个 WS 事件类型。返回取消订阅函数。 */
export function onWsEvent(eventType: string, handler: WsEventHandler): () => void {
  if (!eventListeners.has(eventType)) {
    eventListeners.set(eventType, new Set());
  }
  eventListeners.get(eventType)!.add(handler);
  return () => { eventListeners.get(eventType)?.delete(handler); };
}

/** 向所有订阅者分发一个 WS 事件。由 useWebSocket 调用。 */
export function dispatchWsEvent(eventType: string, data: unknown): void {
  const handlers = eventListeners.get(eventType);
  if (handlers) {
    handlers.forEach((h) => {
      try { h(data); } catch (e) { console.warn('WS event handler error', eventType, e); }
    });
  }
}

// ---------------------------------------------------------------------------
// 向后兼容的薄包装——已有消费者无需修改。
// ---------------------------------------------------------------------------

type TaskChangedListener = (conversationId: string) => void;

export function onTaskChanged(fn: TaskChangedListener): () => void {
  return onWsEvent('task.changed', (data) => {
    fn((data as { conversationId?: string })?.conversationId ?? '');
  });
}

export function notifyTaskChanged(conversationId: string): void {
  dispatchWsEvent('task.changed', { conversationId });
}

// conversation.role_changed 事件载荷：服务端在群聊内 Agent 角色变更后广播。
export interface RoleChangedPayload {
  conversationId: string;
  agentId: string;
  role: string;
  actorId: string;
  demotedAgentId?: string;
}

type RoleChangedListener = (payload: RoleChangedPayload) => void;

export function onConversationRoleChanged(fn: RoleChangedListener): () => void {
  return onWsEvent('conversation.role_changed', (data) => {
    fn(data as RoleChangedPayload);
  });
}

export function notifyConversationRoleChanged(payload: RoleChangedPayload): void {
  dispatchWsEvent('conversation.role_changed', payload);
}

export interface TypingUser {
  userId: string;
  username?: string;
}

interface WsState {
  status: WsStatus;
  wsClient: WebSocketClient | null;
  currentToken: string | null;
  /** conversationId → typing users */
  typingUsers: Record<string, TypingUser[]>;
  /** conversationId → whether an agent is currently processing */
  agentTyping: Record<string, boolean>;
  connect: (token: string) => WebSocketClient | null;
  disconnect: () => void;
  addTypingUser: (conversationId: string, userId: string, username?: string) => void;
  removeTypingUser: (conversationId: string, userId: string) => void;
  setAgentTyping: (conversationId: string, typing: boolean) => void;
}

export const useWsStore = create<WsState>((set, get) => ({
  status: 'disconnected',
  wsClient: null,
  currentToken: null,
  typingUsers: {},
  agentTyping: {},

  connect: (token: string) => {
    // 避免重复连接
    const existing = get().wsClient;
    // React StrictMode 会重复执行 effect，同 token 连接直接复用，避免关闭尚未握手完成的 WebSocket。
    if (existing && get().currentToken === token && get().status !== 'disconnected') {
      return existing;
    }
    if (existing) {
      existing.disconnect();
    }

    const client = new WebSocketClient();
    client.onStatusChange((status) => {
      if (status === 'disconnected') {
        // WebSocket 断开时清除所有 agentTyping 状态，防止残留"正在思考"指示器
        set((prev) => {
          const hasActive = Object.values(prev.agentTyping).some(Boolean);
          return hasActive ? { status, agentTyping: {} } : { status };
        });
      } else {
        set({ status });
      }
    });
    client.connect(token);
    set({ wsClient: client, currentToken: token, status: 'connecting' });
    return client;
  },

  disconnect: () => {
    const client = get().wsClient;
    if (client) {
      client.disconnect();
    }
    agentTypingTimers.forEach((t) => clearTimeout(t));
    agentTypingTimers.clear();
    set({ wsClient: null, currentToken: null, status: 'disconnected', typingUsers: {}, agentTyping: {} });
  },

  addTypingUser: (conversationId, userId, username) => {
    set((state) => {
      const current = state.typingUsers[conversationId] ?? [];
      if (current.some((u) => u.userId === userId)) return state;
      return {
        typingUsers: {
          ...state.typingUsers,
          [conversationId]: [...current, { userId, username }],
        },
      };
    });
  },

  removeTypingUser: (conversationId, userId) => {
    set((state) => {
      const current = state.typingUsers[conversationId] ?? [];
      return {
        typingUsers: {
          ...state.typingUsers,
          [conversationId]: current.filter((u) => u.userId !== userId),
        },
      };
    });
  },

  setAgentTyping: (conversationId, typing) => {
    set((state) => ({
      agentTyping: {
        ...state.agentTyping,
        [conversationId]: typing,
      },
    }));
    // Auto-clear after 60s to handle cases where typing_stop is never received
    if (typing) {
      // Clear previous timer to prevent accumulation
      const prev = agentTypingTimers.get(conversationId);
      if (prev) clearTimeout(prev);
      const timer = setTimeout(() => {
        const current = useWsStore.getState().agentTyping[conversationId];
        if (current) {
          useWsStore.getState().setAgentTyping(conversationId, false);
        }
        agentTypingTimers.delete(conversationId);
      }, 60_000);
      agentTypingTimers.set(conversationId, timer);
    }
  },
}));
