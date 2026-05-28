package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// MessageService 消息服务所需的仓库接口
type MsgRepo interface {
	Create(ctx context.Context, conversationID, role, content, artifactsJSON string) (*model.Message, error)
	ListByConversation(ctx context.Context, conversationID string, before time.Time, limit int) ([]model.Message, error)
}

// ConvRepoForMsg 消息服务需要的对话仓库接口（用于权限校验和更新时间戳）
type ConvRepoForMsg interface {
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	UpdateTimestamp(ctx context.Context, id string) error
}

// AgentRepoForMsg 消息服务查询 Agent 用于对话接入。
type AgentRepoForMsg interface {
	GetByID(ctx context.Context, id string) (*model.Agent, error)
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt string) (*model.DaemonTask, error)
	GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error)
}

var (
	ErrMsgConvNotFound = errors.New("对话不存在")
	ErrMsgConvNoPerm   = errors.New("无权操作此对话")
	ErrMsgAgentNoPerm  = errors.New("无权使用此 Agent")
	ErrMsgAgentOffline = errors.New("Agent 未连接电脑，无法执行真实 CLI")
	ErrMsgAgentTimeout = errors.New("Agent 执行超时")
)

// MessageService 消息业务逻辑
type MessageService struct {
	msgRepo   MsgRepo
	convRepo  ConvRepoForMsg
	agentRepo AgentRepoForMsg
}

// SendMessageResult 发送消息后的用户消息和可选 Agent 回复。
type SendMessageResult struct {
	UserMessage  *model.Message `json:"user_message"`
	AgentMessage *model.Message `json:"agent_message,omitempty"`
}

// NewMessageService 创建消息服务
func NewMessageService(msgRepo MsgRepo, convRepo ConvRepoForMsg, agentRepo AgentRepoForMsg) *MessageService {
	return &MessageService{msgRepo: msgRepo, convRepo: convRepo, agentRepo: agentRepo}
}

// SendMessage 发送消息并刷新对话时间戳
func (s *MessageService) SendMessage(ctx context.Context, convID, userID, role, content, artifactsJSON, agentID string) (*SendMessageResult, error) {
	// 校验对话存在且属于当前用户
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if conv.UserID != userID {
		return nil, ErrMsgConvNoPerm
	}

	if role == "" {
		role = "user"
	}

	msg, err := s.msgRepo.Create(ctx, convID, role, content, artifactsJSON)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	result := &SendMessageResult{UserMessage: msg}
	if strings.TrimSpace(agentID) != "" {
		agentMsg, err := s.createAgentReply(ctx, convID, userID, agentID, content)
		if err != nil {
			return nil, err
		}
		result.AgentMessage = agentMsg
	}

	// 刷新对话的 updated_at
	if err := s.convRepo.UpdateTimestamp(ctx, convID); err != nil {
		return nil, fmt.Errorf("update conversation timestamp: %w", err)
	}

	return result, nil
}

// GetHistory 获取对话消息历史，支持 before 游标分页
func (s *MessageService) GetHistory(ctx context.Context, convID, userID string, before time.Time, limit int) ([]model.Message, error) {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if conv.UserID != userID {
		return nil, ErrMsgConvNoPerm
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	messages, err := s.msgRepo.ListByConversation(ctx, convID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return messages, nil
}

func (s *MessageService) createAgentReply(ctx context.Context, convID, userID, agentID, userContent string) (*model.Message, error) {
	if s.agentRepo == nil {
		return nil, ErrAgentNotFound
	}
	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	if agent.UserID != nil && *agent.UserID != userID {
		return nil, ErrMsgAgentNoPerm
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return nil, ErrMsgAgentOffline
	}

	task, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, userContent)
	if err != nil {
		return nil, fmt.Errorf("create daemon task: %w", err)
	}
	task, err = s.waitDaemonTask(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	if task.Status == "failed" {
		return nil, fmt.Errorf("daemon task failed: %s", task.Error)
	}

	artifacts, err := json.Marshal(map[string]string{
		"agent_id":   agent.ID,
		"agent_name": agent.Name,
		"cli_tool":   agent.CLITool,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal agent message artifacts: %w", err)
	}

	msg, err := s.msgRepo.Create(ctx, convID, "assistant", task.Result, string(artifacts))
	if err != nil {
		return nil, fmt.Errorf("create agent reply: %w", err)
	}
	return msg, nil
}

func (s *MessageService) waitDaemonTask(ctx context.Context, taskID string) (*model.DaemonTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	ticker := time.NewTicker(600 * time.Millisecond)
	defer ticker.Stop()

	for {
		task, err := s.agentRepo.GetDaemonTask(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("get daemon task: %w", err)
		}
		if task != nil && (task.Status == "completed" || task.Status == "failed") {
			return task, nil
		}

		select {
		case <-ctx.Done():
			return nil, ErrMsgAgentTimeout
		case <-ticker.C:
		}
	}
}
