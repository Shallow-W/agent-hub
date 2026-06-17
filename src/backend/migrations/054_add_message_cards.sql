-- messages.cards_json: 存储交互式卡片（plan/progress/confirm/result 等）。
-- Agent 在 task.complete 返回的结构化 JSON 卡片由 daemon 解析后存入此处。
-- 前端通过 CardRegistry 按 type 派发渲染组件。
ALTER TABLE messages ADD COLUMN IF NOT EXISTS cards_json TEXT NOT NULL DEFAULT '';
