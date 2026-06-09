-- 为群聊会话添加头像和简介字段
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS avatar VARCHAR(255) DEFAULT '';
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS description TEXT DEFAULT '';

---- DOWN
ALTER TABLE conversations DROP COLUMN IF EXISTS description;
ALTER TABLE conversations DROP COLUMN IF EXISTS avatar;
