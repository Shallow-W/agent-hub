import { useEffect } from 'react';
import { useAuthStore } from '@/store/authStore';
import { useWsStore } from '@/store/wsStore';

export function useAuth() {
  const user = useAuthStore((s) => s.user);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const loading = useAuthStore((s) => s.loading);
  const error = useAuthStore((s) => s.error);
  const login = useAuthStore((s) => s.login);
  const register = useAuthStore((s) => s.register);
  const logout = useAuthStore((s) => s.logout);
  const loadFromStorage = useAuthStore((s) => s.loadFromStorage);

  useEffect(() => {
    loadFromStorage();
  }, [loadFromStorage]);

  const handleLogout = () => {
    useWsStore.getState().disconnect();
    logout();
    // 清除所有用户相关的 localStorage 键
    localStorage.removeItem('agenthub_active_conv');
    localStorage.removeItem('agenthub_direct_agent_chats');
    // 强制刷新页面，彻底清空所有 zustand 内存状态
    window.location.href = '/login';
  };

  return {
    user,
    isAuthenticated,
    loading,
    error,
    login,
    register,
    logout: handleLogout,
  };
}
