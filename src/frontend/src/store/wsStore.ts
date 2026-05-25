import { create } from 'zustand';
import { WebSocketClient, type WsStatus } from '@/api/websocket';

interface WsState {
  status: WsStatus;
  wsClient: WebSocketClient | null;
  connect: (token: string) => WebSocketClient | null;
  disconnect: () => void;
}

export const useWsStore = create<WsState>((set, get) => ({
  status: 'disconnected',
  wsClient: null,

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
}));
