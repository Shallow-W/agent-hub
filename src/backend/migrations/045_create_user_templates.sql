CREATE TABLE IF NOT EXISTS user_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    content JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_templates_user_type_name ON user_templates(user_id, type, name);
CREATE INDEX IF NOT EXISTS idx_user_templates_user_type ON user_templates(user_id, type, updated_at DESC);
