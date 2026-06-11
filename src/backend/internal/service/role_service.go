package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/agent-hub/backend/internal/domain"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/port"
)

// RoleConvRepo 是 RoleService 依赖的会话仓库接口（窄于 ConvRepo）。
// 仅声明角色变更所需方法，便于将来替换实现或在测试中注入 fake。
type RoleConvRepo interface {
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	ListMemberIDs(ctx context.Context, conversationID string) ([]string, error)
	GetOrchestrator(ctx context.Context, conversationID string) (*model.ConversationAgent, error)
	UpdateAgentRole(ctx context.Context, conversationID, agentID, role string) error
}

// RoleService 独立负责会话内 Agent 角色变更。
//
// 从 ConversationService 拆出：原本的 SetConversationAgentRole 把"权限校验 + 角色切换 + 事件广播"
// 三件事揉在一起，且依赖 WS 推送能力。抽出后：
//   - ConversationService 只管对话/成员 CRUD；
//   - RoleService 专注角色语义（orchestrator 唯一性、降级、广播）。
type RoleService struct {
	convRepo RoleConvRepo
	events   port.EventBroadcaster
}

// NewRoleService 创建 RoleService。events 可为 nil（仅用于测试场景），
// 此时角色仍会变更但不广播事件。
func NewRoleService(convRepo RoleConvRepo, events port.EventBroadcaster) *RoleService {
	return &RoleService{convRepo: convRepo, events: events}
}

// Set 设置会话中某 Agent 的角色。
//
// 行为：
//   - role 必须是可赋值角色（orchestrator / worker），否则 ErrConvInvalidRole；
//   - 仅群聊创建者 / owner / admin 可操作；
//   - 设为 orchestrator 时，先把现有 orchestrator 降级为 worker（若非同一 Agent）；
//   - 成功后通过 EventBroadcaster 向所有会话成员推送 role_changed 事件，让其他客户端实时刷新。
//
// 广播失败仅记录日志，不影响主流程（角色已持久化）。
func (s *RoleService) Set(ctx context.Context, userID, convID, agentID string, role domain.Role) error {
	if !domain.IsAssignableRole(role) {
		return ErrConvInvalidRole
	}
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrConvNotFound
	}
	if err := canManageConversationAgents(ctx, s.convRepo, userID, conv); err != nil {
		return err
	}

	// 设为 Orch 时，先把现有 Orch 降级为 Worker（orchestrator 唯一性约束）。
	demotedAgentID := ""
	if role == domain.RoleOrchestrator {
		current, err := s.convRepo.GetOrchestrator(ctx, convID)
		if err != nil {
			return fmt.Errorf("get current orchestrator: %w", err)
		}
		if current != nil && current.AgentID != agentID {
			if err := s.convRepo.UpdateAgentRole(ctx, convID, current.AgentID, string(domain.RoleWorker)); err != nil {
				return fmt.Errorf("demote old orchestrator: %w", err)
			}
			demotedAgentID = current.AgentID
		}
	}
	if err := s.convRepo.UpdateAgentRole(ctx, convID, agentID, string(role)); err != nil {
		// 仅在写 Orchestrator 时检查 DB 部分唯一索引冲突（migration 047）。
		// 这是 RoleService "先查后写" 的并发兜底：两个并发 Set 都读到无 orch，
		// 都尝试写新 orch，第二个会被 uq_conversation_agents_single_orchestrator 拒绝。
		if role == domain.RoleOrchestrator && isUniqueViolation(err) {
			return ErrConvOrchConflict
		}
		return fmt.Errorf("update agent role: %w", err)
	}

	// 角色已成功持久化，广播事件失败不影响主流程。
	if s.events == nil {
		return nil
	}
	memberIDs, err := s.convRepo.ListMemberIDs(ctx, convID)
	if err != nil {
		slog.Warn("role changed: list members failed, skip broadcast",
			"conv_id", convID, "actor_id", userID, "error", err)
		return nil
	}
	event := port.RoleEvent{
		ConversationID: convID,
		ActorID:        userID,
		AgentID:        agentID,
		Role:           role,
		DemotedAgentID: demotedAgentID,
	}
	if err := s.events.BroadcastRoleChanged(ctx, memberIDs, event); err != nil {
		slog.Warn("role changed: broadcast failed",
			"conv_id", convID, "actor_id", userID, "error", err)
	}
	return nil
}
