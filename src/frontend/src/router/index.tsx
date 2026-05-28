import React from 'react';
import {
  createBrowserRouter,
  Navigate,
  type RouteObject,
} from 'react-router-dom';
import AppLayout from '@/layout/AppLayout';
import { useAuthStore } from '@/store/authStore';
import LoginView from '@/views/LoginView';
import RegisterView from '@/views/RegisterView';
import ChatView from '@/views/ChatView';
import NotFoundView from '@/views/NotFoundView';
import SettingsView from '@/views/SettingsView';

/** 检查是否已登录，未登录则重定向到登录页 */
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

/** 已登录用户访问登录/注册页时重定向到首页 */
function PublicOnlyRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  if (isAuthenticated) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
}

const routes: RouteObject[] = [
  {
    path: '/login',
    element: <PublicOnlyRoute><LoginView /></PublicOnlyRoute>,
  },
  {
    path: '/register',
    element: <PublicOnlyRoute><RegisterView /></PublicOnlyRoute>,
  },
  {
    path: '/',
    element: (
      <ProtectedRoute>
        <AppLayout />
      </ProtectedRoute>
    ),
    children: [
      {
        index: true,
        element: <ChatView />,
      },
      {
        path: 'settings',
        element: <SettingsView />,
      },
    ],
  },
  {
    path: '*',
    element: <NotFoundView />,
  },
];

export const router = createBrowserRouter(routes);
