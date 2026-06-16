import React, { lazy, Suspense } from 'react';
import { Spin } from 'antd';
import {
  createBrowserRouter,
  Navigate,
  type RouteObject,
} from 'react-router-dom';
import AppLayout from '@/layout/AppLayout';
import { useAuthStore } from '@/store/authStore';
import LoginView from '@/views/LoginView';
import RegisterView from '@/views/RegisterView';
import NotFoundView from '@/views/NotFoundView';

const ChatView = lazy(() => import('@/views/ChatView'));
const ContactsView = lazy(() => import('@/views/ContactsView'));
const AgentsView = lazy(() => import('@/views/AgentsView'));
const SkillsView = lazy(() => import('@/views/SkillsView'));
const KnowledgeView = lazy(() => import('@/views/KnowledgeView'));
const TaskBoardView = lazy(() => import('@/views/TaskBoardView'));
const SettingsView = lazy(() => import('@/views/SettingsView'));

const withSuspense = (el: React.ReactNode) => (
  <Suspense fallback={<div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}><Spin /></div>}>
    {el}
  </Suspense>
);

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
      { index: true, element: withSuspense(<ChatView />) },
      { path: 'contacts', element: withSuspense(<ContactsView />) },
      { path: 'agents', element: withSuspense(<AgentsView />) },
      { path: 'skills', element: withSuspense(<SkillsView />) },
      { path: 'knowledge', element: withSuspense(<KnowledgeView />) },
      { path: 'tasks', element: withSuspense(<TaskBoardView />) },
      { path: 'settings', element: withSuspense(<SettingsView />) },
    ],
  },
  {
    path: '*',
    element: <NotFoundView />,
  },
];

export const router = createBrowserRouter(routes);
