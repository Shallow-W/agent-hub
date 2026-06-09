# Frontend Quality Guidelines

> 前端开发硬性规则与禁止模式。

---

## 硬性规则

### 类型安全
- **禁止使用 `any`**，用 `unknown` 或具体类型替代

### API 调用
- 所有 REST 请求通过 `api/` 模块发出，组件内**禁止直接调用** `fetch`/`axios`

### WebSocket
- WebSocket 通过自定义 Hook 消费，**不直接操作** WebSocket 实例

### 样式
- 样式使用 **CSS Modules**，类名 **camelCase**
- **禁止内联样式**
