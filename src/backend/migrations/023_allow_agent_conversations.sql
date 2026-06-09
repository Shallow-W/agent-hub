-- 允许智能体一对一会话作为独立 conversation 类型。
ALTER TABLE conversations DROP CONSTRAINT IF EXISTS chk_conversations_type;
ALTER TABLE conversations ADD CONSTRAINT chk_conversations_type
  CHECK (type IN ('single', 'group', 'agent'));

---- DOWN
UPDATE conversations SET type = 'group' WHERE type = 'agent';
ALTER TABLE conversations DROP CONSTRAINT IF EXISTS chk_conversations_type;
ALTER TABLE conversations ADD CONSTRAINT chk_conversations_type
  CHECK (type IN ('single', 'group'));
