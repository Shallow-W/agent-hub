import { create } from 'zustand';
import { WebSocketClient, type WsStatus } from '@/api/websocket';

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
      setTimeout(() => {
        const current = useWsStore.getState().agentTyping[conversationId];
        if (current) {
          useWsStore.getState().setAgentTyping(conversationId, false);
        }
      }, 60_000);
    }
  },
}));
