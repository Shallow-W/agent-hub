import { create } from 'zustand';
import type { User } from '@/types/auth';
import * as authApi from '@/api/auth';
import * as userApi from '@/api/user';
import { setToken, clearToken } from '@/api/client';
import { STORAGE_KEYS } from '@/config/constants';
import { resetConversationStore } from '@/store/conversationStore';
import { resetMessageStore } from '@/store/messageStore';
import { resetAgentStore } from '@/store/agentStore';
import { resetFriendStore } from '@/store/friendStore';
import { resetKnowledgeStore } from '@/store/knowledgeStore';
import { resetCatalogStore } from '@/store/catalogStore';
import { useWsStore } from '@/store/wsStore';

interface AuthState {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  loading: boolean;
  error: string | null;
  login: (username: string, password: string) => Promise<void>;
  register: (username: string, password: string) => Promise<void>;
  updateAvatar: (avatar: string) => Promise<void>;
  updateUsername: (username: string) => Promise<void>;
  logout: () => void;
  loadFromStorage: () => void;
}




export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  token: null,
  isAuthenticated: false,
  loading: false,
  error: null,

  login: async (username: string, password: string) => {
    set({ loading: true, error: null });
    try {
      const data = await authApi.login(username, password);
      localStorage.setItem(STORAGE_KEYS.TOKEN, data.token);
      localStorage.setItem(STORAGE_KEYS.USER, JSON.stringify(data.user));
      localStorage.removeItem(STORAGE_KEYS.ACTIVE_CONV);
      setToken(data.token);
      resetAgentStore();
      set({ user: data.user, token: data.token, isAuthenticated: true });
    } catch (err) {
      const msg = err instanceof Error ? err.message : '登录失败';
      set({ error: msg });
      throw err;
    } finally {
      set({ loading: false });
    }
  },

  register: async (username: string, password: string) => {
    set({ loading: true, error: null });
    try {
      const data = await authApi.register(username, password);
      localStorage.setItem(STORAGE_KEYS.TOKEN, data.token);
      localStorage.setItem(STORAGE_KEYS.USER, JSON.stringify(data.user));
      localStorage.removeItem(STORAGE_KEYS.ACTIVE_CONV);
      setToken(data.token);
      resetAgentStore();
      set({ user: data.user, token: data.token, isAuthenticated: true });
    } catch (err) {
      const msg = err instanceof Error ? err.message : '注册失败';
      set({ error: msg });
      throw err;
    } finally {
      set({ loading: false });
    }
  },

  updateAvatar: async (avatar: string) => {
    const updated = await userApi.updateUserAvatar(avatar);
    const current = get().user;
    // 合并：以服务端返回为准，兜底保留本地已有字段。
    const next: User = { ...(current ?? {} as User), ...updated };
    localStorage.setItem(STORAGE_KEYS.USER, JSON.stringify(next));
    set({ user: next });
  },

  updateUsername: async (username: string) => {
    const updated = await userApi.updateUsername(username);
    const current = get().user;
    const next: User = { ...(current ?? {} as User), ...updated };
    localStorage.setItem(STORAGE_KEYS.USER, JSON.stringify(next));
    set({ user: next });
  },

  logout: () => {
    localStorage.removeItem(STORAGE_KEYS.TOKEN);
    localStorage.removeItem(STORAGE_KEYS.USER);
    localStorage.removeItem(STORAGE_KEYS.ACTIVE_CONV);
    clearToken();
    resetConversationStore();
    resetMessageStore();
    resetAgentStore();
    resetFriendStore();
    resetKnowledgeStore();
    resetCatalogStore();
    useWsStore.getState().disconnect();
    set({ user: null, token: null, isAuthenticated: false });
  },

  loadFromStorage: () => {
    const token = localStorage.getItem(STORAGE_KEYS.TOKEN);
    const userJson = localStorage.getItem(STORAGE_KEYS.USER);
    if (token && userJson) {
      try {
        const user: User = JSON.parse(userJson);
        setToken(token);
        set({ user, token, isAuthenticated: true });
      } catch {
        // 数据损坏则清除
        localStorage.removeItem(STORAGE_KEYS.TOKEN);
        localStorage.removeItem(STORAGE_KEYS.USER);
      }
    }
  },
}));
