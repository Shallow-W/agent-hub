package port

import (
	"context"

	"github.com/agent-hub/backend/internal/domain"
)

// RoleEvent 表示一次会话内 Agent 角色变更事件。
type RoleEvent struct {
	ConversationID  string      // 会话 ID
	ActorID         string      // 触发变更的用户 ID
	AgentID         string      // 被改角色的 Agent ID
	Role            domain.Role // 新角色
	DemotedAgentID  string      // 若 orch 切换，旧 orch 的 Agent ID；空表示无降级
}

// EventBroadcaster 把领域事件推送给在线用户。
// 实现方负责把事件转换为 WSMessage 并按 memberIDs 单播/广播。
type EventBroadcaster interface {
	BroadcastRoleChanged(ctx context.Context, memberIDs []string, event RoleEvent) error
}
