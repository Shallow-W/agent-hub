-- 本地/远端电脑连接表
CREATE TABLE IF NOT EXISTS daemon_machines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    api_key_hash VARCHAR(64) UNIQUE NOT NULL,
    machine_id VARCHAR(120) NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE agents ADD COLUMN IF NOT EXISTS machine_id UUID REFERENCES daemon_machines(id) ON DELETE SET NULL;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS machine_name VARCHAR(100) NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_agents_user_name;
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_user_name ON agents (user_id, name)
    WHERE user_id IS NOT NULL AND machine_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_daemon_machines_user ON daemon_machines (user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_agents_machine ON agents (machine_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_machine_cli ON agents (machine_id, cli_tool) WHERE machine_id IS NOT NULL;

---- DOWN
DROP INDEX IF EXISTS idx_agents_machine_cli;
DROP INDEX IF EXISTS idx_agents_machine;
DROP INDEX IF EXISTS idx_daemon_machines_user;
DROP INDEX IF EXISTS idx_agents_user_name;
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_user_name ON agents (user_id, name) WHERE user_id IS NOT NULL;
ALTER TABLE agents DROP COLUMN IF EXISTS machine_name;
ALTER TABLE agents DROP COLUMN IF EXISTS machine_id;
DROP TABLE IF EXISTS daemon_machines;
