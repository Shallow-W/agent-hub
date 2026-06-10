import { create } from 'zustand';
import type { User } from '@/types/auth';
import * as authApi from '@/api/auth';
import * as userApi from '@/api/user';
import { setToken, clearToken } from '@/api/client';
import { resetConversationStore } from '@/store/conversationStore';
import { resetMessageStore } from '@/store/messageStore';
import { resetAgentStore } from '@/store/agentStore';
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

const TOKEN_KEY = 'agenthub_token';
const USER_KEY = 'agenthub_user';

function clearPersistedAuth() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

function saveSessionUser(user: User) {
  sessionStorage.setItem(USER_KEY, JSON.stringify(user));
}

function clearSessionAuth() {
  sessionStorage.removeItem(TOKEN_KEY);
  sessionStorage.removeItem(USER_KEY);
}

function parseSessionUser(raw: string | null): User | null {
  if (!raw) {
    return null;
  }

  try {
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== 'object') {
      return null;
    }
    const user = value as Record<string, unknown>;
    if (
      typeof user.id === 'string'
      && typeof user.username === 'string'
      && typeof user.created_at === 'string'
      && (user.avatar === undefined || typeof user.avatar === 'string')
    ) {
      return {
        id: user.id,
        username: user.username,
        avatar: user.avatar,
        created_at: user.created_at,
      };
    }
  } catch {
    return null;
  }

  return null;
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
      clearPersistedAuth();
      localStorage.removeItem('agenthub_active_conv');
      setToken(data.token);
      saveSessionUser(data.user);
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
      clearPersistedAuth();
      localStorage.removeItem('agenthub_active_conv');
      setToken(data.token);
      saveSessionUser(data.user);
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
    const next: User = { ...(current ?? {} as User), ...updated };
    saveSessionUser(next);
    set({ user: next });
  },

  updateUsername: async (username: string) => {
    const updated = await userApi.updateUsername(username);
    const current = get().user;
    const next: User = { ...(current ?? {} as User), ...updated };
    saveSessionUser(next);
    set({ user: next });
  },

  logout: () => {
    clearPersistedAuth();
    clearSessionAuth();
    localStorage.removeItem('agenthub_active_conv');
    clearToken();
    resetConversationStore();
    resetMessageStore();
    resetAgentStore();
    useWsStore.getState().disconnect();
    set({ user: null, token: null, isAuthenticated: false });
  },

  loadFromStorage: () => {
    clearPersistedAuth();
    const token = sessionStorage.getItem(TOKEN_KEY);
    const user = parseSessionUser(sessionStorage.getItem(USER_KEY));
    if (!token || !user) {
      clearSessionAuth();
      clearToken();
      set({ user: null, token: null, isAuthenticated: false });
      return;
    }
    setToken(token);
    set({ user, token, isAuthenticated: true });
  },
}));
