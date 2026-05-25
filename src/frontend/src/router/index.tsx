import React from 'react';
import {
  createBrowserRouter,
  Navigate,
  type RouteObject,
} from 'react-router-dom';
import AppLayout from '@/layout/AppLayout';
import LoginView from '@/views/LoginView';
import RegisterView from '@/views/RegisterView';
import ChatView from '@/views/ChatView';

/** 检查是否已登录，未登录则重定向到登录页 */
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const token = localStorage.getItem('agenthub_token');
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

const routes: RouteObject[] = [
  {
    path: '/login',
    element: <LoginView />,
  },
  {
    path: '/register',
    element: <RegisterView />,
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
    ],
  },
];

export const router = createBrowserRouter(routes);
