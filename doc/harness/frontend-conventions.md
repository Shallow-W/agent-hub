# 前端编码规范（React + TypeScript）

## 通用规则

- **注释语言**：中文（说明"为什么"，而非"做了什么"）
- **命名语言**：英文（变量、函数、类、文件名）
- **缩进**：2空格
- **编码**：UTF-8
- **换行**：LF（非CRLF），通过 `.editorconfig` 和 `.gitattributes` 保证

---

## 文件命名

| 类型 | 命名格式 | 示例 |
|------|----------|------|
| 页面组件 | PascalCase（XxxView） | `ChatView.tsx` |
| UI组件文件 | PascalCase | `ChatWindow.tsx` |
| 布局组件 | PascalCase（XxxLayout） | `AppLayout.tsx` |
| 自定义Hook | camelCase（use前缀） | `useWebSocket.ts` |
| API模块 | camelCase | `conversation.ts` |
| 类型定义 | camelCase | `message.ts` |
| 工具函数 | camelCase | `formatMessage.ts` |
| 样式文件 | 与组件同名 | `ChatWindow.module.css` |
| 测试文件 | 组件名.test | `ChatWindow.test.tsx` |

## 组件规范

```tsx
// 优先使用函数式组件 + Hooks
const ChatWindow: React.FC<ChatWindowProps> = ({ conversationId }) => {
  // 1. Hooks（useState, useEffect, 自定义Hooks）
  // 2. 事件处理函数
  // 3. 渲染逻辑

  return (
    <div className={styles.container}>
      {/* JSX */}
    </div>
  );
};
```

- 组件用 `React.FC<Props>` 类型
- Props 接口定义在组件文件内，命名 `{ComponentName}Props`
- 单个组件文件不超过 300 行，超过则拆分子组件
- 提取自定义 Hook 复用有状态逻辑

## TypeScript 规范

- 严格模式开启（`strict: true`）
- 禁止使用 `any`，用 `unknown` 替代或定义具体类型
- 接口（interface）优先，类型别名（type）用于联合类型/工具类型
- 枚举使用 `const enum` 或字符串字面量联合类型

## 状态管理

- 组件内状态：`useState` / `useReducer`
- 跨组件共享：通过 Context 或轻量状态库
- 服务端状态（对话、消息）：通过 API 层获取和缓存

## API 调用规范

- 所有 REST 请求统一通过 `api/` 模块发出，组件内禁止直接调用 `fetch`/`axios`
- API 函数返回类型化的 `Promise`，错误通过统一的错误类型抛出

```ts
// api/conversation.ts
export async function getConversation(id: string): Promise<Conversation> {
  const res = await fetch(`/api/conversations/${id}`);
  if (!res.ok) throw new ApiError(res.status, await res.json());
  return res.json();
}
```

- 组件内通过 `try/catch` 或 Hook 包装处理错误，UI 层展示错误提示

## WebSocket 使用规范

- WebSocket 连接管理集中在 `api/websocket.ts`，对外暴露事件订阅接口
- 组件通过自定义 Hook（如 `useWebSocket`）消费消息，不直接操作 WebSocket 实例

```ts
// 组件内使用方式
const { messages, send, status } = useWebSocket(conversationId);
```

- 自动重连策略：断开后指数退避重连（1s → 2s → 4s → 最大 30s）
- 流式消息（Agent 回复）通过 `message.streaming` 事件增量拼接，收到 `done: true` 后写入最终状态

## 导入顺序

```ts
// 1. React / 第三方库
import React, { useState } from 'react';
import { useParams } from 'react-router-dom';

// 2. 项目内部模块（使用 @/ 别名）
import { getConversation } from '@/api/conversation';
import { ChatMessage } from '@/types/message';

// 3. 相对路径（同目录或子组件）
import { MessageBubble } from './MessageBubble';

// 4. 样式
import styles from './ChatWindow.module.css';
```

## 样式方案

- 使用 CSS Modules（`*.module.css`）
- 类名使用 camelCase
- 避免内联样式

## 技术选型

| 类别 | 选型 | 备注 |
|------|------|------|
| 构建工具 | Vite | |
| 状态管理 | Zustand | 轻量，适合中小规模 |
| 路由 | React Router v6 | |
| HTTP 客户端 | 原生 fetch | 封装在 api/ 模块 |
| 样式 | CSS Modules | |

---

## 路由设计

```
/login                  ← 登录/注册页（独立布局，无侧边栏）
/register               ← 注册页

/                       ← 主布局（侧边栏 + 内容区）
├── /                   ← 重定向到 /chat
├── /chat               ← 对话列表（侧边栏）+ 聊天窗口（内容区）
├── /chat/:conversationId  ← 指定对话的聊天窗口
├── /agents             ← Agent 管理列表
├── /agents/new         ← 创建自建 Agent
└── /settings           ← 个人设置
```

## 页面与布局

### 布局结构

项目有两种布局，通过 React Router 的 `<Outlet>` 嵌套：

```
AuthLayout（登录/注册）
└── 全屏居中卡片，无导航

AppLayout（主界面）
├── Sidebar（左侧固定）
│   ├── 用户头像 + 状态
│   ├── 导航菜单（对话 / Agent / 设置）
│   └── 对话搜索
└── <Outlet />（右侧内容区）
    ├── ChatView         → ChatWindow
    ├── AgentsView       → AgentList
    └── SettingsView     → SettingsForm
```

### 页面与组件对应

| 路由 | 页面组件 | 主要子组件 |
|------|----------|-----------|
| `/login` | `LoginView` | `LoginForm` |
| `/register` | `RegisterView` | `RegisterForm` |
| `/chat` | `ChatView` | `ConversationList`（侧边栏）+ `EmptyState`（内容区） |
| `/chat/:id` | `ChatView` | `ConversationList`（侧边栏）+ `ChatWindow` |
| `/agents` | `AgentsView` | `AgentList`、`AgentCard` |
| `/agents/new` | `AgentCreateView` | `AgentCreator` |
| `/settings` | `SettingsView` | `SettingsForm` |

### 组件目录组织

```
src/
├── assets/                # 静态资源（图片、字体、图标）
├── components/            # 通用/可复用组件（按业务域组织）
│   ├── common/            #   基础UI组件（Button/Input/Modal/Avatar）
│   ├── chat/              #   聊天相关（ChatWindow/MessageList/MessageBubble/ChatInput）
│   ├── sidebar/           #   侧边栏（Sidebar/ConversationList/NavMenu）
│   ├── agent/             #   Agent管理（AgentCard/AgentList/AgentCreator）
│   └── preview/           #   产物预览（CodeCard/WebpageCard/FileCard）
├── views/                 # 页面组件（与路由 1:1 对应）
│   ├── LoginView.tsx
│   ├── RegisterView.tsx
│   ├── ChatView.tsx
│   ├── AgentsView.tsx
│   ├── AgentCreateView.tsx
│   └── SettingsView.tsx
├── layout/                # 布局组件
│   ├── AuthLayout.tsx
│   └── AppLayout.tsx
├── router/                # 路由配置
│   └── index.tsx
├── store/                 # 状态管理（Zustand）
├── api/                   # API 封装（REST + WebSocket）
│   ├── conversation.ts
│   ├── message.ts
│   ├── agent.ts
│   ├── auth.ts
│   └── websocket.ts
├── hooks/                 # 自定义 Hooks
│   ├── useWebSocket.ts
│   ├── useConversation.ts
│   └── useAuth.ts
├── types/                 # 全局 TypeScript 类型定义
│   ├── message.ts
│   ├── conversation.ts
│   ├── agent.ts
│   └── artifact.ts
├── utils/                 # 通用工具方法
├── styles/                # 全局样式
├── App.tsx
└── main.tsx
```
