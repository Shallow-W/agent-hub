-- 会话中的机器人成员：把已创建 Agent 作为“好友式 Robot”加入某个对话。
CREATE TABLE IF NOT EXISTS conversation_agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    added_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL DEFAULT 'robot',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_conversation_agent UNIQUE (conversation_id, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_conversation_agents_conv
    ON conversation_agents (conversation_id, joined_at ASC);
CREATE INDEX IF NOT EXISTS idx_conversation_agents_agent
    ON conversation_agents (agent_id);

---- DOWN
DROP INDEX IF EXISTS idx_conversation_agents_agent;
DROP INDEX IF EXISTS idx_conversation_agents_conv;
DROP TABLE IF EXISTS conversation_agents;
