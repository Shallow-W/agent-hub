-- messages.blocks_json: 存储 assistant 消息的完整 block 结构（text/thinking/tool_use/tool_result）。
-- 流式期间 daemon 通过 task.progress 增量更新，task.complete 时 backend 把累积的 blocks 落库。
-- 历史消息加载时优先用 blocks_json 做 block 渲染，为空时 fallback 到 content（markdown）。
ALTER TABLE messages ADD COLUMN IF NOT EXISTS blocks_json TEXT NOT NULL DEFAULT '';

-- messages.status: 消息生命周期状态。
--   streaming  — 流式生成中（预创建的 assistant message）
--   complete   — 正常完成（默认）
--   error      — 执行失败（保留 content/blocks_json 已输出部分 + error block）
--   canceled   — 用户主动取消（SIGINT）
-- 旧消息默认 complete，查询时默认过滤掉 streaming/error 以保持列表整洁。
ALTER TABLE messages ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'complete';

-- streaming 状态的消息通常很少（并发上限），单独索引加速 watchdog 扫描。
CREATE INDEX IF NOT EXISTS idx_messages_streaming ON messages(status) WHERE status = 'streaming';
