-- Add sender_id to messages for multi-user IM support
ALTER TABLE messages ADD COLUMN IF NOT EXISTS sender_id UUID REFERENCES users(id);

-- Backfill: for existing messages with role='user', set sender_id to the conversation creator
UPDATE messages m
SET sender_id = c.user_id
FROM conversations c
WHERE m.conversation_id = c.id AND m.role = 'user' AND m.sender_id IS NULL;

---- DOWN
ALTER TABLE messages DROP COLUMN IF EXISTS sender_id;
