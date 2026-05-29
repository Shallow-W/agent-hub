import { create } from 'zustand';
import type { Friend, FriendRequest } from '@/types/friend';
import type { User } from '@/types/auth';
import * as friendApi from '@/api/friend';

interface FriendState {
  friends: Friend[];
  pendingRequests: FriendRequest[];
  loading: boolean;
  sending: boolean;
  error: string | null;
  searchResults: User[];
  isSearching: boolean;
  /** Tracks which request ID is currently being accepted/rejected */
  actionLoading: string | null;

  fetchFriends: () => Promise<void>;
  fetchPending: () => Promise<void>;
  sendRequest: (username: string) => Promise<void>;
  acceptRequest: (id: string) => Promise<void>;
  rejectRequest: (id: string) => Promise<void>;
  searchUsers: (username: string) => Promise<void>;
  clearSearch: () => void;
  deleteFriend: (friendId: string) => Promise<void>;
}

export const useFriendStore = create<FriendState>((set) => ({
  friends: [],
  pendingRequests: [],
  loading: false,
  sending: false,
  error: null,
  searchResults: [],
  isSearching: false,
  actionLoading: null,

  fetchFriends: async () => {
    set({ loading: true, error: null });
    try {
      const list = await friendApi.listFriends();
      set({ friends: list ?? [] });
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
      set({ pendingRequests: list ?? [] });
    } catch {
      // 静默失败，不影响主流程
    }
  },

  sendRequest: async (username: string) => {
    set({ sending: true, error: null });
    try {
      await friendApi.sendFriendRequest(username);
      await Promise.all([
        friendApi.listFriends(),
        friendApi.listPendingRequests(),
      ]).then(([friends, pending]) => {
        set({ friends: friends ?? [], pendingRequests: pending ?? [] });
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : '发送请求失败';
      set({ error: msg });
      throw err;
    } finally {
      set({ sending: false });
    }
  },

  acceptRequest: async (id: string) => {
    set({ actionLoading: id, error: null });
    try {
      await friendApi.acceptFriendRequest(id);
      // 刷新列表
      const [friends, pending] = await Promise.all([
        friendApi.listFriends(),
        friendApi.listPendingRequests(),
      ]);
      set({ friends: friends ?? [], pendingRequests: pending ?? [], error: null });
    } catch (err) {
      const msg = err instanceof Error ? err.message : '操作失败';
      set({ error: msg });
    } finally {
      set({ actionLoading: null });
    }
  },

  rejectRequest: async (id: string) => {
    set({ actionLoading: id, error: null });
    try {
      await friendApi.rejectFriendRequest(id);
      const pending = await friendApi.listPendingRequests();
      set({ pendingRequests: pending ?? [], error: null });
    } catch (err) {
      const msg = err instanceof Error ? err.message : '操作失败';
      set({ error: msg });
    } finally {
      set({ actionLoading: null });
    }
  },

  searchUsers: async (username: string) => {
    set({ isSearching: true });
    try {
      const results = await friendApi.searchUsers(username);
      set({ searchResults: results ?? [] });
    } catch {
      set({ searchResults: [] });
    } finally {
      set({ isSearching: false });
    }
  },

  clearSearch: () => {
    set({ searchResults: [], isSearching: false });
  },

  deleteFriend: async (friendId: string) => {
    try {
      await friendApi.deleteFriend(friendId);
      set((state) => ({
        friends: state.friends.filter((f) => f.friend_id !== friendId),
      }));
    } catch (err) {
      const msg = err instanceof Error ? err.message : '删除好友失败';
      set({ error: msg });
      throw err;
    }
  },
}));
