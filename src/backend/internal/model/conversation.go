package model

import (
	"time"

	"github.com/agent-hub/backend/internal/domain"
)

// Conversation 对话模型
type Conversation struct {
	ID           string     `json:"id" db:"id"`
	UserID       string     `json:"user_id" db:"user_id"`
	Type         string     `json:"type" db:"type"`
	Title        string     `json:"title" db:"title"`
	Avatar       string     `json:"avatar,omitempty" db:"avatar"`
	Description  string     `json:"description,omitempty" db:"description"`
	Announcement string     `json:"announcement,omitempty" db:"announcement"`
	Tags         string     `json:"tags,omitempty" db:"tags"`
	Pinned       bool       `json:"pinned" db:"pinned"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty" db:"archived_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`

	// 计算字段，非 DB 列
	PeerID      string `json:"peer_id,omitempty" db:"peer_id"`
	PeerName    string `json:"peer_name,omitempty" db:"peer_name"`
	LastMessage string `json:"last_message,omitempty" db:"last_message"`
	MemberCount int    `json:"member_count,omitempty" db:"member_count"`
}

// ConversationAgent 表示某个对话中已加入的 Robot 成员。
type ConversationAgent struct {
	ID               string     `json:"id" db:"id"`
	ConversationID   string     `json:"conversation_id" db:"conversation_id"`
	AgentID          string     `json:"agent_id" db:"agent_id"`
	AddedBy          string     `json:"added_by" db:"added_by"`
	Role             string     `json:"role" db:"role"`
	JoinedAt         time.Time  `json:"joined_at" db:"joined_at"`
	Name             string     `json:"name" db:"name"`
	Type             string     `json:"type" db:"type"`
	CLITool          string     `json:"cli_tool" db:"cli_tool"`
	Avatar           string     `json:"avatar" db:"avatar"`
	Source           string     `json:"source" db:"source"`
	Status           string     `json:"status" db:"status"`
	Version          string     `json:"version" db:"version"`
	MachineID        *string    `json:"machine_id,omitempty" db:"machine_id"`
	MachineName      string     `json:"machine_name" db:"machine_name"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty" db:"last_seen_at"`
	CapabilitiesJSON string     `json:"capabilities_json" db:"capabilities_json"`
	CustomSkills     string     `json:"custom_skills,omitempty" db:"custom_skills"`
	SystemPrompt     string     `json:"system_prompt,omitempty" db:"system_prompt"`
	Description      string     `json:"description,omitempty" db:"description"`
	Tags             string     `json:"tags,omitempty" db:"tags"`
}

// ConversationAgents 是会话 Agent 列表的领域别名，
// 便于在 service / orchestrator 层使用 FindByAgentID / Orchestrator / Workers 等方法。
type ConversationAgents []ConversationAgent

// IsOrchestrator 判断该会话 Agent 是否为 Orchestrator 角色。
func (ca ConversationAgent) IsOrchestrator() bool {
	return domain.Role(ca.Role) == domain.RoleOrchestrator
}

// IsWorker 判断该会话 Agent 是否为 Worker 角色。
func (ca ConversationAgent) IsWorker() bool {
	return domain.Role(ca.Role) == domain.RoleWorker
}

// FindByAgentID 在列表中按 Agent ID 查找会话 Agent。
// 找不到时返回 (nil, false)。
func (cas ConversationAgents) FindByAgentID(agentID string) (*ConversationAgent, bool) {
	for i := range cas {
		if cas[i].AgentID == agentID {
			return &cas[i], true
		}
	}
	return nil, false
}

// Orchestrator 返回列表中的 Orchestrator 角色 Agent。
// 一个会话同一时刻至多存在一个 Orchestrator，找不到时返回 (nil, false)。
func (cas ConversationAgents) Orchestrator() (*ConversationAgent, bool) {
	for i := range cas {
		if cas[i].IsOrchestrator() {
			return &cas[i], true
		}
	}
	return nil, false
}

// Workers 返回列表中所有 Worker 角色 Agent。
func (cas ConversationAgents) Workers() []ConversationAgent {
	out := make([]ConversationAgent, 0, len(cas))
	for i := range cas {
		if cas[i].IsWorker() {
			out = append(out, cas[i])
		}
	}
	return out
}
