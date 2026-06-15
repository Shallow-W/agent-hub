ALTER TABLE platform_skills
    ADD COLUMN IF NOT EXISTS category VARCHAR(80) NOT NULL DEFAULT '未分类';

CREATE INDEX IF NOT EXISTS idx_platform_skills_user_category ON platform_skills(user_id, category, updated_at DESC);
