-- Agent 配置表
CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(20) NOT NULL DEFAULT 'system',
    cli_tool VARCHAR(50) NOT NULL,
    system_prompt TEXT NOT NULL DEFAULT '',
    avatar VARCHAR(255) NOT NULL DEFAULT '',
    capabilities_json TEXT NOT NULL DEFAULT '',
    source VARCHAR(20) NOT NULL DEFAULT 'manual',
    status VARCHAR(20) NOT NULL DEFAULT 'offline',
    version VARCHAR(100) NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agents_user_type ON agents (user_id, type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_system_cli ON agents (cli_tool) WHERE user_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_user_name ON agents (user_id, name) WHERE user_id IS NOT NULL;

---- DOWN
DROP TABLE IF EXISTS agents;
