-- tool_categories: lookup table for MCP tool categories (label / color / sort order).
-- Drives the frontend tool catalog UI; populated once and read-only from app code.
CREATE TABLE IF NOT EXISTS tool_categories (
    name VARCHAR(32) PRIMARY KEY,
    label VARCHAR(32) NOT NULL,
    color VARCHAR(8) NOT NULL,
    sort_order INT NOT NULL DEFAULT 0
);

INSERT INTO tool_categories (name, label, color, sort_order) VALUES
    ('conversation', '会话', '#1677ff', 1),
    ('task', '任务', '#fa8c16', 2),
    ('agent', 'Agent', '#722ed1', 3),
    ('machine', '电脑', '#595959', 4),
    ('group', '群聊', '#52c41a', 5),
    ('skill', '技能', '#eb2f96', 6),
    ('knowledge', '知识库', '#13c2c2', 7),
    ('deployment', '部署', '#722ed1', 8)
ON CONFLICT (name) DO NOTHING;
