import { create } from 'zustand';
import { WebSocketClient, type WsStatus } from '@/api/websocket';

const agentTypingTimers = new Map<string, ReturnType<typeof setTimeout>>();

type TaskChangedListener = (conversationId: string) => void;
const taskChangedListeners = new Set<TaskChangedListener>();

export function onTaskChanged(fn: TaskChangedListener): () => void {
  taskChangedListeners.add(fn);
  return () => { taskChangedListeners.delete(fn); };
}

export function notifyTaskChanged(conversationId: string): void {
  taskChangedListeners.forEach((fn) => fn(conversationId));
}

// conversation.role_changed 事件载荷：服务端在群聊内 Agent 角色变更后广播。
// 任何订阅者（如 GroupMemberPanel）收到事件后可按 conversationId 决定是否刷新本地视图。
export interface RoleChangedPayload {
  conversationId: string;
  agentId: string;
  role: string;
  actorId: string;
  demotedAgentId?: string;
}

type RoleChangedListener = (payload: RoleChangedPayload) => void;
const roleChangedListeners = new Set<RoleChangedListener>();

export function onConversationRoleChanged(fn: RoleChangedListener): () => void {
  roleChangedListeners.add(fn);
  return () => { roleChangedListeners.delete(fn); };
}

export function notifyConversationRoleChanged(payload: RoleChangedPayload): void {
  roleChangedListeners.forEach((fn) => fn(payload));
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
