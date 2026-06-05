# 基础 P1 产物预览收口

## Goal

完成基础 P1 产物预览体验：Agent 回复里的代码、网页、HTML Preview、文档类文本产物都能内联展示，并能展开到全屏预览或轻量代码编辑器。P2 的 PPT、Diff、版本历史和对话式局部修改不在本轮实现。

## Requirements

- 代码块在消息正文中高亮显示，支持复制和展开。
- 代码块展开后可编辑，支持内部滚动、复制编辑后的内容、重置为原始内容。
- 网页 URL 和 HTML 产物以内联卡片展示，点击后进入大预览。
- 网页 iframe 支持常见站内跳转；HTML srcdoc 支持脚本、弹窗、表单和用户触发跳转。
- 文档类产物以内联卡片展示，点击后进入全屏文档预览。
- 文档类 MVP 支持 markdown/text/json/csv/html 这类文本内容；PDF/PPT/二进制文件先给出清晰兜底。

## Acceptance Criteria

- [ ] 代码展开后黑色编辑区接近全屏，可编辑、可滚动、可复制编辑后的内容。
- [ ] 网页/HTML Preview 内联卡片和全屏预览可用。
- [ ] 文档类文本产物可在聊天中出现卡片，并可全屏阅读。
- [ ] 不支持的文档类型显示明确兜底，不白屏。
- [ ] 纯文本回复不出现空产物卡片。
- [ ] `npm run build` 通过。

## Out of Scope

- PPT 浏览。
- Diff 视图。
- 版本历史。
- 将编辑内容写回历史消息或生成新版本。
- 选中代码后对话式局部修改。

## Technical Notes

- 前端主要涉及 `src/frontend/src/components/chat/ArtifactCard.tsx`、`ArtifactWorkspace.tsx`、`CodeBlock.tsx` 及对应 CSS Modules。
- 后端已有 `model.Artifact` 和消息 `Artifacts` 字段，本轮优先不改存储结构。
