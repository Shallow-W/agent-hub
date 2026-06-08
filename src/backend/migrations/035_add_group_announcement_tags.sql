-- 为群聊会话添加公告和标签字段
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS announcement TEXT DEFAULT '';
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS tags TEXT DEFAULT '';

---- DOWN
ALTER TABLE conversations DROP COLUMN IF EXISTS tags;
ALTER TABLE conversations DROP COLUMN IF EXISTS announcement;
