import { create } from 'zustand';
import { WebSocketClient, type WsStatus } from '@/api/websocket';

interface WsState {
  status: WsStatus;
  wsClient: WebSocketClient | null;
  /** conversationId → typing user IDs */
  typingUsers: Record<string, string[]>;
  connect: (token: string) => WebSocketClient | null;
  disconnect: () => void;
  addTypingUser: (conversationId: string, userId: string) => void;
  removeTypingUser: (conversationId: string, userId: string) => void;
}

export const useWsStore = create<WsState>((set, get) => ({
  status: 'disconnected',
  wsClient: null,
  typingUsers: {},

  connect: (token: string) => {
    // 避免重复连接
    const existing = get().wsClient;
    if (existing) {
      existing.disconnect();
    }

    const client = new WebSocketClient();
    client.onStatusChange((status) => {
      set({ status });
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
    set({ wsClient: null, status: 'disconnected' });
  },

  addTypingUser: (conversationId, userId) => {
    set((state) => {
      const current = state.typingUsers[conversationId] ?? [];
      if (current.includes(userId)) return state;
      return {
        typingUsers: {
          ...state.typingUsers,
          [conversationId]: [...current, userId],
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
          [conversationId]: current.filter((id) => id !== userId),
        },
      };
    });
  },
}));
