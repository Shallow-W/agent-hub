package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/golang-jwt/jwt/v5"
)

// AgentRepo Agent 服务所需仓库接口
type AgentRepo interface {
	ListAvailable(ctx context.Context, userID string) ([]model.Agent, error)
	GetByID(ctx context.Context, id string) (*model.Agent, error)
	GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error)
	ClaimDaemonTask(ctx context.Context, machineID string) (*model.DaemonTask, error)
	CompleteDaemonTask(ctx context.Context, id, machineID, result, taskError string) (bool, error)
	UpsertSystemAgent(ctx context.Context, name, cliTool, version, capabilitiesJSON string) error
	CreateDaemonMachine(ctx context.Context, userID, name, apiKeyHash string) (*model.DaemonMachine, error)
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error)
	ListDaemonMachines(ctx context.Context, userID string) ([]model.DaemonMachine, error)
	DeleteDaemonMachine(ctx context.Context, id, userID string) (bool, error)
	GetDaemonMachineByAPIKeyHash(ctx context.Context, apiKeyHash string) (*model.DaemonMachine, error)
	GetDaemonMachineByID(ctx context.Context, id string) (*model.DaemonMachine, error)
	MarkDaemonMachineConnected(ctx context.Context, id, machineID string) error
	UpsertMachineAgent(ctx context.Context, userID, machineID, machineName, name, cliTool, version, capabilitiesJSON string) error
	UpsertMachineAgentCandidate(ctx context.Context, machineID, name, cliTool, version, capabilitiesJSON string) error
	ListAgentCandidates(ctx context.Context, userID string) ([]model.AgentCandidate, error)
	AddCandidateAgent(ctx context.Context, userID, candidateID, displayName, systemPrompt string) (*model.Agent, error)
	CreateCustom(ctx context.Context, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON string, enableManagementTools bool) (*model.Agent, error)
	UpdateCustom(ctx context.Context, id, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON string, enableManagementTools bool) (*model.Agent, error)
	UpdateAgentStatus(ctx context.Context, id, status string) error
	ClearAgentMachine(ctx context.Context, id string) error
	UpdateMachineAPIKey(ctx context.Context, id, apiKeyHash string) error
	DeleteOwned(ctx context.Context, id, userID string) (bool, error)
}

// ClaimDaemonTask 为当前已认证电脑领取一条待执行任务。
func (s *AgentService) ClaimDaemonTask(ctx context.Context, machine *model.DaemonMachine) (*model.DaemonTask, error) {
	if machine == nil || machine.ID == "" {
		return nil, ErrAgentInvalidInput
	}
	task, err := s.repo.ClaimDaemonTask(ctx, machine.ID)
	if err != nil {
		return nil, fmt.Errorf("claim daemon task: %w", err)
	}
	return task, nil
}

// CompleteDaemonTask 接收当前电脑执行 CLI 后的真实结果。
func (s *AgentService) CompleteDaemonTask(ctx context.Context, machine *model.DaemonMachine, taskID, result, taskError string) error {
	if machine == nil || machine.ID == "" || taskID == "" {
		return ErrAgentInvalidInput
	}
	ok, err := s.repo.CompleteDaemonTask(ctx, taskID, machine.ID, result, taskError)
	if err != nil {
		return fmt.Errorf("complete daemon task: %w", err)
	}
	if !ok {
		return ErrAgentNotFound
	}
	return nil
}

var (
	ErrAgentNotFound     = errors.New("Agent 不存在")
	ErrAgentInvalidInput = errors.New("Agent 参数无效")
)

const machineAPIKeyPrefix = "sk_machine_"

// AgentService Agent 管理业务逻辑
type AgentService struct {
	repo     AgentRepo
	tracker  *MachineTracker
	jwtSecret string
}

// DiscoveredAgent 是 daemon 上报的本机 Agent 摘要
type DiscoveredAgent struct {
	Name         string            `json:"name"`
	CLITool      string            `json:"cli_tool"`
	Version      string            `json:"version"`
	Capabilities []DiscoveredSkill `json:"capabilities"`
}

// NewAgentService 创建 Agent 服务
func NewAgentService(repo AgentRepo, tracker *MachineTracker) *AgentService {
	return &AgentService{repo: repo, tracker: tracker}
}

// SetJWTSecret 设置 JWT 密钥（用于生成 Agent Token）
func (s *AgentService) SetJWTSecret(secret string) {
	s.jwtSecret = secret
}

// ListAvailable 查询当前用户可用 Agent
func (s *AgentService) ListAvailable(ctx context.Context, userID string) ([]model.Agent, error) {
	list, err := s.repo.ListAvailable(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	return list, nil
}

// TouchMachine 更新机器心跳（内存操作，零 DB 开销）。
func (s *AgentService) TouchMachine(machineID string) {
	if s.tracker != nil {
		s.tracker.Touch(machineID)
	}
}

// MarkMachineOnline 标记机器上线（daemon register 时调用）。
func (s *AgentService) MarkMachineOnline(machineID string) {
	if s.tracker != nil {
		s.tracker.MarkOnline(machineID)
	}
}

// IsMachineOnline 检查机器是否在线（内存读取）。
func (s *AgentService) IsMachineOnline(machineID string) bool {
	if s.tracker != nil {
		return s.tracker.IsOnline(machineID)
	}
	return false
}

// RegisterSystemAgents 保存 daemon 上报的系统 Agent
func (s *AgentService) RegisterSystemAgents(ctx context.Context, agents []DiscoveredAgent) error {
	for _, agent := range agents {
		name := strings.TrimSpace(agent.Name)
		cliTool := strings.TrimSpace(agent.CLITool)
		if name == "" || cliTool == "" {
			continue
		}
		capabilities, err := json.Marshal(agent.Capabilities)
		if err != nil {
			return fmt.Errorf("marshal capabilities: %w", err)
		}
		if err := s.repo.UpsertSystemAgent(ctx, name, cliTool, agent.Version, string(capabilities)); err != nil {
			return fmt.Errorf("upsert system agent: %w", err)
		}
	}
	return nil
}

// CreateDaemonMachine 创建一台等待 daemon 连接的电脑，并返回明文 machine key。
func (s *AgentService) CreateDaemonMachine(ctx context.Context, userID, name string) (*model.DaemonMachine, string, error) {
	name = strings.TrimSpace(name)
	if userID == "" || name == "" {
		return nil, "", ErrAgentInvalidInput
	}

	apiKey, err := generateMachineAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("generate machine api key: %w", err)
	}
	machine, err := s.repo.CreateDaemonMachine(ctx, userID, name, hashMachineAPIKey(apiKey))
	if err != nil {
		return nil, "", fmt.Errorf("create daemon machine: %w", err)
	}
	return machine, apiKey, nil
}

// ListDaemonMachines 查询用户已经创建的电脑连接位。
func (s *AgentService) ListDaemonMachines(ctx context.Context, userID string) ([]model.DaemonMachine, error) {
	list, err := s.repo.ListDaemonMachines(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list daemon machines: %w", err)
	}
	return list, nil
}

// DeleteDaemonMachine 删除当前用户创建的电脑连接。
func (s *AgentService) DeleteDaemonMachine(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrAgentInvalidInput
	}
	deleted, err := s.repo.DeleteDaemonMachine(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("delete daemon machine: %w", err)
	}
	if !deleted {
		return ErrAgentNotFound
	}
	return nil
}

// GetDaemonMachineByAPIKey 根据明文 machine key 查找对应电脑。
func (s *AgentService) GetDaemonMachineByAPIKey(ctx context.Context, apiKey string) (*model.DaemonMachine, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" || !strings.HasPrefix(apiKey, machineAPIKeyPrefix) {
		return nil, ErrAgentInvalidInput
	}
	machine, err := s.repo.GetDaemonMachineByAPIKeyHash(ctx, hashMachineAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("get daemon machine by api key: %w", err)
	}
	if machine == nil {
		return nil, ErrAgentNotFound
	}
	return machine, nil
}

// RegisterMachineAgents 保存指定电脑 daemon 上报的候选 Agent。
func (s *AgentService) RegisterMachineAgents(ctx context.Context, machine *model.DaemonMachine, machineHostID string, agents []DiscoveredAgent) error {
	if machine == nil || machine.ID == "" || machine.UserID == "" {
		return ErrAgentInvalidInput
	}
	machineHostID = strings.TrimSpace(machineHostID)
	if machineHostID == "" {
		machineHostID = "unknown-machine"
	}
	if err := s.repo.MarkDaemonMachineConnected(ctx, machine.ID, machineHostID); err != nil {
		return fmt.Errorf("mark daemon machine connected: %w", err)
	}

	for _, agent := range agents {
		name := strings.TrimSpace(agent.Name)
		cliTool := strings.TrimSpace(agent.CLITool)
		if name == "" || cliTool == "" {
			continue
		}
		capabilities, err := json.Marshal(agent.Capabilities)
		if err != nil {
			return fmt.Errorf("marshal capabilities: %w", err)
		}
		if err := s.repo.UpsertMachineAgentCandidate(
			ctx,
			machine.ID,
			name,
			cliTool,
			agent.Version,
			string(capabilities),
		); err != nil {
			return fmt.Errorf("upsert machine agent: %w", err)
		}
	}
	return nil
}

// ListAgentCandidates 查询当前用户电脑上检测到的候选 Agent。
func (s *AgentService) ListAgentCandidates(ctx context.Context, userID string) ([]model.AgentCandidate, error) {
	list, err := s.repo.ListAgentCandidates(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list agent candidates: %w", err)
	}
	return list, nil
}

// AddCandidateAgent 将候选 Agent 添加成可用 Agent。
func (s *AgentService) AddCandidateAgent(ctx context.Context, userID, candidateID, displayName, systemPrompt string) (*model.Agent, error) {
	displayName = strings.TrimSpace(displayName)
	systemPrompt = strings.TrimSpace(systemPrompt)
	if userID == "" || candidateID == "" || displayName == "" {
		return nil, ErrAgentInvalidInput
	}
	agent, err := s.repo.AddCandidateAgent(ctx, userID, candidateID, displayName, systemPrompt)
	if err != nil {
		return nil, fmt.Errorf("add candidate agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	return agent, nil
}

// CreateCustom 创建自建 Agent
func (s *AgentService) CreateCustom(ctx context.Context, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON string, enableManagementTools bool) (*model.Agent, error) {
	name = strings.TrimSpace(name)
	cliTool = strings.TrimSpace(cliTool)
	if name == "" || cliTool == "" {
		return nil, ErrAgentInvalidInput
	}
	agent, err := s.repo.CreateCustom(ctx, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, enableManagementTools)
	if err != nil {
		return nil, fmt.Errorf("create custom agent: %w", err)
	}
	return agent, nil
}

// UpdateCustom 更新自建 Agent
func (s *AgentService) UpdateCustom(ctx context.Context, id, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON string, enableManagementTools bool) (*model.Agent, error) {
	name = strings.TrimSpace(name)
	cliTool = strings.TrimSpace(cliTool)
	if id == "" || name == "" || cliTool == "" {
		return nil, ErrAgentInvalidInput
	}
	if err := syncSkillFiles(capabilitiesJSON); err != nil {
		return nil, err
	}
	agent, err := s.repo.UpdateCustom(ctx, id, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, enableManagementTools)
	if err != nil {
		return nil, fmt.Errorf("update custom agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	return agent, nil
}

// DeleteOwned 删除当前用户拥有的 Agent。
func (s *AgentService) DeleteOwned(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrAgentInvalidInput
	}
	deleted, err := s.repo.DeleteOwned(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("delete owned agent: %w", err)
	}
	if !deleted {
		return ErrAgentNotFound
	}
	return nil
}

// GenerateAgentToken 生成 Agent 专用 JWT，带 agent_management scope，有效期 5 分钟。
func (s *AgentService) GenerateAgentToken(ctx context.Context, userID string) (string, time.Time, error) {
	if userID == "" {
		return "", time.Time{}, ErrAgentInvalidInput
	}
	now := time.Now()
	expiresAt := now.Add(5 * time.Minute)
	claims := jwt.MapClaims{
		"user_id": userID,
		"scope":   "agent_management",
		"iat":     now.Unix(),
		"exp":     expiresAt.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign agent token: %w", err)
	}
	return tokenStr, expiresAt, nil
}

// RestartAgent 重启 Agent：通过 daemon task 通知远端电脑重新启动对应 CLI 进程。
func (s *AgentService) RestartAgent(ctx context.Context, agentID, userID string) error {
	if agentID == "" || userID == "" {
		return ErrAgentInvalidInput
	}
	agent, err := s.repo.GetByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return ErrAgentNotFound
	}
	// 校验所有权：系统 Agent（user_id 为 nil）对所有用户可见
	if agent.UserID != nil && *agent.UserID != userID {
		return ErrAgentNotFound
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return errors.New("agent offline")
	}
	// 创建重启任务，conversation_id 和 prompt 留空，仅作为控制指令
	_, err = s.repo.CreateDaemonTask(ctx, userID, "", agentID, *agent.MachineID, agent.CLITool, "__restart__", "")
	if err != nil {
		return fmt.Errorf("create restart task: %w", err)
	}
	return nil
}

// StopAgent 停止 Agent：将状态设为 offline 并清除 machine_id。
func (s *AgentService) StopAgent(ctx context.Context, agentID, userID string) error {
	if agentID == "" || userID == "" {
		return ErrAgentInvalidInput
	}
	agent, err := s.repo.GetByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return ErrAgentNotFound
	}
	if agent.UserID != nil && *agent.UserID != userID {
		return ErrAgentNotFound
	}
	return s.repo.ClearAgentMachine(ctx, agentID)
}

// GetMachineConnectCommand 获取电脑连接命令。需要重新生成 API Key（原始密钥只存储哈希）。
func (s *AgentService) GetMachineConnectCommand(ctx context.Context, machineID, userID string) (string, *model.DaemonMachine, error) {
	if machineID == "" || userID == "" {
		return "", nil, ErrAgentInvalidInput
	}
	machine, err := s.repo.GetDaemonMachineByID(ctx, machineID)
	if err != nil {
		return "", nil, fmt.Errorf("get daemon machine: %w", err)
	}
	if machine == nil || machine.UserID != userID {
		return "", nil, ErrAgentNotFound
	}
	// 生成新的 API Key 并更新哈希
	apiKey, err := generateMachineAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("generate machine api key: %w", err)
	}
	if err := s.updateMachineAPIKey(ctx, machineID, hashMachineAPIKey(apiKey)); err != nil {
		return "", nil, fmt.Errorf("update machine api key: %w", err)
	}
	// 更新 machine 对象以反映新 key（但 APIKeyHash 不返回给前端）
	machine.APIKeyHash = ""
	command := fmt.Sprintf("npx @agenthub/daemon --server-url http://localhost:8080 --api-key %s", apiKey)
	return command, machine, nil
}

// updateMachineAPIKey 更新电脑的 API Key 哈希。
func (s *AgentService) updateMachineAPIKey(ctx context.Context, machineID, apiKeyHash string) error {
	// 通过 repo 接口需要新增方法，这里直接用已有的 repo 内部机制
	return s.repo.UpdateMachineAPIKey(ctx, machineID, apiKeyHash)
}

func generateMachineAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return machineAPIKeyPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashMachineAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}
