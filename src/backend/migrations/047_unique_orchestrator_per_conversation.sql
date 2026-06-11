-- 单 Orch 唯一性：每个会话至多一个 role='orchestrator' 的 Agent。
-- 应用层 RoleService 已经做了"先 demote 旧 orch 再写新 orch"的逻辑，
-- 此部分唯一索引作为并发兜底，消除两个并发请求同时设 orch 的 race window。
-- 不约束 role='worker' 或 role='robot'（默认值）。

-- 历史脏数据兜底：如果存在同一会话多个 orchestrator 的旧数据，
-- 只保留最早的（joined_at 最小者），其余降级为 worker，避免 CREATE UNIQUE INDEX 失败。
UPDATE conversation_agents ca
SET role = 'worker'
WHERE role = 'orchestrator'
  AND joined_at > (
      SELECT MIN(joined_at)
      FROM conversation_agents ca2
      WHERE ca2.conversation_id = ca.conversation_id
        AND ca2.role = 'orchestrator'
  );

CREATE UNIQUE INDEX IF NOT EXISTS uq_conversation_agents_single_orchestrator
    ON conversation_agents (conversation_id)
    WHERE role = 'orchestrator';

---- DOWN
DROP INDEX IF EXISTS uq_conversation_agents_single_orchestrator;
