CREATE TABLE IF NOT EXISTS builtin_skill_templates (
    name VARCHAR(64) PRIMARY KEY,
    label VARCHAR(64) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    skill_categories JSONB NOT NULL DEFAULT '[]'::jsonb
);

INSERT INTO builtin_skill_templates (name, label, description, skill_categories) VALUES
    ('none', '无技能', '不分配任何平台 Skill', '[]'::jsonb),
    ('pm', '产品经理', '产品需求、PRD、验收标准', '["产品经理"]'::jsonb),
    ('dev', '开发人员', '技术方案、代码实现、代码审查', '["开发人员"]'::jsonb)
ON CONFLICT (name) DO NOTHING;
