-- 同一台电脑可以基于同一个 CLI 创建多个 Agent 实例
DROP INDEX IF EXISTS idx_agents_machine_cli;
CREATE INDEX IF NOT EXISTS idx_agents_machine_cli ON agents (machine_id, cli_tool)
    WHERE machine_id IS NOT NULL;

---- DOWN
DROP INDEX IF EXISTS idx_agents_machine_cli;
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_machine_cli ON agents (machine_id, cli_tool)
    WHERE machine_id IS NOT NULL;
