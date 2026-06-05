-- 给 daemon_tasks 增加 context_messages 字段，存储最近 40 条对话上下文（JSON）
ALTER TABLE daemon_tasks ADD COLUMN IF NOT EXISTS context_messages TEXT NOT NULL DEFAULT '';

---- DOWN
ALTER TABLE daemon_tasks DROP COLUMN IF EXISTS context_messages;
