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
| 组件文件 | PascalCase | `ChatWindow.tsx` |
| 工具函数 | camelCase | `formatMessage.ts` |
| 类型定义 | PascalCase | `Message.ts` |
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

## 样式方案

- 使用 CSS Modules（`*.module.css`）
- 类名使用 camelCase
- 避免内联样式
