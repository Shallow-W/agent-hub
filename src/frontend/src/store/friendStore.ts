import { create } from 'zustand';
import type { Friend, FriendRequest } from '@/types/friend';
import * as friendApi from '@/api/friend';

interface FriendState {
  friends: Friend[];
  pendingRequests: FriendRequest[];
  loading: boolean;
  error: string | null;

  fetchFriends: () => Promise<void>;
  fetchPending: () => Promise<void>;
  sendRequest: (username: string) => Promise<void>;
  acceptRequest: (id: string) => Promise<void>;
  rejectRequest: (id: string) => Promise<void>;
}

export const useFriendStore = create<FriendState>((set) => ({
  friends: [],
  pendingRequests: [],
  loading: false,
  error: null,

  fetchFriends: async () => {
    set({ loading: true, error: null });
    try {
      const list = await friendApi.listFriends();
      set({ friends: list });
    } catch (err) {
      const msg = err instanceof Error ? err.message : '获取好友列表失败';
      set({ error: msg });
    } finally {
      set({ loading: false });
    }
  },

  fetchPending: async () => {
    try {
      const list = await friendApi.listPendingRequests();
      set({ pendingRequests: list });
    } catch {
      // 静默失败，不影响主流程
    }
  },

  sendRequest: async (username: string) => {
    set({ loading: true, error: null });
    try {
      await friendApi.sendFriendRequest(username);
      await Promise.all([
        friendApi.listFriends(),
        friendApi.listPendingRequests(),
      ]).then(([friends, pending]) => {
        set({ friends, pendingRequests: pending });
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : '发送请求失败';
      set({ error: msg });
      throw err;
    } finally {
      set({ loading: false });
    }
  },

  acceptRequest: async (id: string) => {
    try {
      await friendApi.acceptFriendRequest(id);
      // 刷新列表
      const [friends, pending] = await Promise.all([
        friendApi.listFriends(),
        friendApi.listPendingRequests(),
      ]);
      set({ friends, pendingRequests: pending });
    } catch (err) {
      const msg = err instanceof Error ? err.message : '操作失败';
      set({ error: msg });
    }
  },

  rejectRequest: async (id: string) => {
    try {
      await friendApi.rejectFriendRequest(id);
      const pending = await friendApi.listPendingRequests();
      set({ pendingRequests: pending });
    } catch (err) {
      const msg = err instanceof Error ? err.message : '操作失败';
      set({ error: msg });
    }
  },
}));
