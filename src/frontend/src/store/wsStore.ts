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

export interface TypingUser {
  userId: string;
  username?: string;
}

interface WsState {
  status: WsStatus;
  wsClient: WebSocketClient | null;
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
  typingUsers: {},
  agentTyping: {},

  connect: (token: string) => {
    // 避免重复连接
    const existing = get().wsClient;
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
    set({ wsClient: client, status: 'connecting' });
    return client;
  },

  disconnect: () => {
    const client = get().wsClient;
    if (client) {
      client.disconnect();
    }
    agentTypingTimers.forEach((t) => clearTimeout(t));
    agentTypingTimers.clear();
    set({ wsClient: null, status: 'disconnected', typingUsers: {}, agentTyping: {} });
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
