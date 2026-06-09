CREATE TABLE IF NOT EXISTS agent_prompt_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    category VARCHAR(80) NOT NULL DEFAULT '通用',
    description TEXT NOT NULL DEFAULT '',
    system_prompt TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_prompt_templates_user_name ON agent_prompt_templates(user_id, name);
CREATE INDEX IF NOT EXISTS idx_agent_prompt_templates_user_category ON agent_prompt_templates(user_id, category, updated_at DESC);
