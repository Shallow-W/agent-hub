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

### 受限高度布局
- 在固定高度或 `overflow: hidden` 面板内使用纵向 flex 分区时，必须明确哪个容器负责滚动
- 可展开分区如果需要保持上下文档流顺序，设置 `flex: 0 0 auto`，并在分区或内部列表上设置 `overflow`
- 避免让被 flex 压缩的 section 继续绘制外溢内容，否则展开第二个分区时可能覆盖第一个分区
