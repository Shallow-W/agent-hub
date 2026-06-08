-- 用户头像：存头像 key（如 user5）或直接 URL；为空时前端按 id 稳定哈希回退到 user1~user20。
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar TEXT NOT NULL DEFAULT '';

---- DOWN
ALTER TABLE users DROP COLUMN IF EXISTS avatar;
