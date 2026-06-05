-- daemon 扫描到的候选 Agent，用户确认后才进入 agents 表
CREATE TABLE IF NOT EXISTS daemon_agent_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    machine_id UUID NOT NULL REFERENCES daemon_machines(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    cli_tool VARCHAR(50) NOT NULL,
    version VARCHAR(100) NOT NULL DEFAULT '',
    capabilities_json TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_daemon_agent_candidates_machine_cli
    ON daemon_agent_candidates (machine_id, cli_tool);
CREATE INDEX IF NOT EXISTS idx_daemon_agent_candidates_machine
    ON daemon_agent_candidates (machine_id, updated_at DESC);

---- DOWN
DROP INDEX IF EXISTS idx_daemon_agent_candidates_machine;
DROP INDEX IF EXISTS idx_daemon_agent_candidates_machine_cli;
DROP TABLE IF EXISTS daemon_agent_candidates;
