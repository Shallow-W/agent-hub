import { create } from 'zustand';
import type { User } from '@/types/auth';
import * as authApi from '@/api/auth';
import * as userApi from '@/api/user';
import { setToken, clearToken } from '@/api/client';
import { resetConversationStore } from '@/store/conversationStore';
import { resetMessageStore } from '@/store/messageStore';
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
  logout: () => void;
  loadFromStorage: () => void;
}

const TOKEN_KEY = 'agenthub_token';
const USER_KEY = 'agenthub_user';

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
      localStorage.setItem(TOKEN_KEY, data.token);
      localStorage.setItem(USER_KEY, JSON.stringify(data.user));
      localStorage.removeItem('agenthub_active_conv');
      setToken(data.token);
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
      localStorage.setItem(TOKEN_KEY, data.token);
      localStorage.setItem(USER_KEY, JSON.stringify(data.user));
      localStorage.removeItem('agenthub_active_conv');
      setToken(data.token);
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
    localStorage.setItem(USER_KEY, JSON.stringify(next));
    set({ user: next });
  },

  logout: () => {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
    localStorage.removeItem('agenthub_active_conv');
    clearToken();
    resetConversationStore();
    resetMessageStore();
    useWsStore.getState().disconnect();
    set({ user: null, token: null, isAuthenticated: false });
  },

  loadFromStorage: () => {
    const token = localStorage.getItem(TOKEN_KEY);
    const userJson = localStorage.getItem(USER_KEY);
    if (token && userJson) {
      try {
        const user: User = JSON.parse(userJson);
        setToken(token);
        set({ user, token, isAuthenticated: true });
      } catch {
        // 数据损坏则清除
        localStorage.removeItem(TOKEN_KEY);
        localStorage.removeItem(USER_KEY);
      }
    }
  },
}));
