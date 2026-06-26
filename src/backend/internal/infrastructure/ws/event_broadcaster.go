package ws

import (
	"context"
	"log/slog"

	"github.com/agent-hub/backend/internal/port"
	pkgws "github.com/agent-hub/backend/pkg/ws"
)

// EventTypeRoleChanged 会话内 Agent 角色变更事件的 WS type。
const EventTypeRoleChanged = "conversation.role_changed"

// EventBroadcaster 实现 port.EventBroadcaster，复用现有 ws.Hub 的事件总线。
//
// 不直接调用 hub.SendToUser：Hub 已有 PushCustomEvent 通道，通过 bus 串行分发，
// 避免和 Register/Unregister 等异步事件产生数据竞争。
type EventBroadcaster struct {
	hub    *pkgws.Hub
	logger *slog.Logger
}

// NewEventBroadcaster 创建一个基于 ws.Hub 的 EventBroadcaster。
func NewEventBroadcaster(hub *pkgws.Hub, logger *slog.Logger) *EventBroadcaster {
	return &EventBroadcaster{hub: hub, logger: logger}
}

// BroadcastRoleChanged 向给定 memberIDs 的所有在线连接推送角色变更事件。
// 离线成员自然丢弃（Hub.PushCustomEvent 内部跳过未注册用户）。
func (b *EventBroadcaster) BroadcastRoleChanged(_ context.Context, memberIDs []string, event port.RoleEvent) error {
	payload := map[string]any{
		"conversation_id":  event.ConversationID,
		"agent_id":         event.AgentID,
		"role":             string(event.Role),
		"actor_id":         event.ActorID,
		"demoted_agent_id": event.DemotedAgentID,
	}
	b.hub.PushCustomEvent(event.ConversationID, memberIDs, EventTypeRoleChanged, payload)
	return nil
}
