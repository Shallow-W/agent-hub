import { create } from 'zustand';
import type { User } from '@/types/auth';
import * as authApi from '@/api/auth';
import { setToken, clearToken } from '@/api/client';

interface AuthState {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  loading: boolean;
  error: string | null;
  login: (username: string, password: string) => Promise<void>;
  register: (username: string, password: string) => Promise<void>;
  logout: () => void;
  loadFromStorage: () => void;
}

const TOKEN_KEY = 'agenthub_token';
const USER_KEY = 'agenthub_user';

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  token: null,
  isAuthenticated: false,
  loading: false,
  error: null,

  login: async (username: string, password: string) => {
    set({ loading: true, error: null });
    try {
      const data = await authApi.login(username, password);
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
      set({ user: data.user, token: data.token, isAuthenticated: true });
    } catch (err) {
      const msg = err instanceof Error ? err.message : '注册失败';
      set({ error: msg });
      throw err;
    } finally {
      set({ loading: false });
    }
  },

  logout: () => {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
    clearToken();
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
