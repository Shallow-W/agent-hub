-- daemon skill 同步任务不属于某个聊天会话，允许 conversation_id 为空。
ALTER TABLE daemon_tasks ALTER COLUMN conversation_id DROP NOT NULL;

---- DOWN
ALTER TABLE daemon_tasks ALTER COLUMN conversation_id SET NOT NULL;
