-- DB-06: conversations.type CHECK constraint
ALTER TABLE conversations ADD CONSTRAINT chk_conversations_type
  CHECK (type IN ('single', 'group'));

-- DB-07: friends.status CHECK constraint
ALTER TABLE friends ADD CONSTRAINT chk_friends_status
  CHECK (status IN ('pending', 'accepted', 'rejected'));

-- DB-11: GroupRepo.AddMember 幂等保护 (补 ON CONFLICT)
-- 已在 Go 代码层通过 AddMember 的 ON CONFLICT DO NOTHING 处理，此处确保唯一约束存在
CREATE UNIQUE INDEX IF NOT EXISTS idx_conv_members_unique
  ON conversation_members (conversation_id, user_id);
