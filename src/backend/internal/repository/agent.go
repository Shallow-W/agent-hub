package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// AgentRepo Agent 数据访问
type AgentRepo struct {
	db *sqlx.DB

	// daemon 任务是实时聊天的临时工单，改存内存而非 DB：消除任务的持久化写入与
	// DB 轮询开销。进程重启会丢弃在途任务（可接受——真正的聊天记录持久化在 messages 表）。
	taskMu    sync.Mutex
	tasks     map[string]*model.DaemonTask
	taskQueue map[string][]string // machineID -> 待领取 taskID FIFO

	dispatchMu sync.RWMutex
	dispatcher func(*model.DaemonTask)
}

// NewAgentRepo 创建 Agent 仓库
func NewAgentRepo(db *sqlx.DB) *AgentRepo {
	return &AgentRepo{
		db:        db,
		tasks:     make(map[string]*model.DaemonTask),
		taskQueue: make(map[string][]string),
	}
}

// SetDaemonTaskDispatcher 注册实时任务投递器，避免 daemon 端轮询后端。
func (r *AgentRepo) SetDaemonTaskDispatcher(dispatcher func(*model.DaemonTask)) {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	r.dispatcher = dispatcher
}

// ListAvailable 查询系统 Agent 和当前用户自建 Agent。userID 为空时返回所有 Agent。
func (r *AgentRepo) ListAvailable(ctx context.Context, userID string) ([]model.Agent, error) {
	list := make([]model.Agent, 0)
	query := `SELECT id, user_id, name, type, cli_tool, system_prompt, tools_config, avatar,
		        capabilities_json, custom_skills, tags, source, status, version, machine_id, machine_name, enable_management_tools,
		        last_seen_at, created_at, updated_at
		 FROM agents`
	var args []interface{}
	if userID != "" {
		query += ` WHERE user_id IS NULL OR user_id = $1`
		args = append(args, userID)
	}
	query += ` ORDER BY type ASC, updated_at DESC`
	err := r.db.SelectContext(ctx, &list, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	return list, nil
}

// UpsertSystemAgent 写入 daemon 上报的系统 Agent。machineID 为空时不绑定电脑。
func (r *AgentRepo) UpsertSystemAgent(ctx context.Context, name, cliTool, version, capabilitiesJSON, machineID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO agents (name, type, cli_tool, capabilities_json, source, status, version, machine_id, last_seen_at)
		 VALUES ($1, 'system', $2, $3, 'daemon', 'online', $4, NULLIF($5,''), NOW())
		 ON CONFLICT (cli_tool) WHERE user_id IS NULL DO UPDATE
		 SET name = EXCLUDED.name,
		     source = 'daemon',
		     status = 'online',
		     version = EXCLUDED.version,
		     machine_id = EXCLUDED.machine_id,
		     last_seen_at = NOW(),
		     updated_at = NOW()`,
		name, cliTool, capabilitiesJSON, version, machineID,
	)
	if err != nil {
		return fmt.Errorf("upsert system agent: %w", err)
	}
	return nil
}

// IsAgentInConversation 校验 Agent 是否已作为 Robot 加入当前用户的对话。
func (r *AgentRepo) IsAgentInConversation(ctx context.Context, conversationID, agentID, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowxContext(ctx,
		`SELECT EXISTS (
		   SELECT 1
		   FROM conversation_agents ca
		   JOIN conversations c ON c.id = ca.conversation_id
		   JOIN agents a ON a.id = ca.agent_id
		   WHERE ca.conversation_id = $1
		     AND ca.agent_id = $2
		     AND (c.user_id = $3 OR EXISTS (
		       SELECT 1 FROM conversation_members cm
		       WHERE cm.conversation_id = c.id AND cm.user_id = $3
		     ))
		     AND (a.user_id IS NULL OR a.user_id = $3)
		 )`,
		conversationID, agentID, userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check conversation agent: %w", err)
	}
	return exists, nil
}

// GetByID 按 ID 查询 Agent
func (r *AgentRepo) GetByID(ctx context.Context, id string) (*model.Agent, error) {
	var a model.Agent
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, user_id, name, type, cli_tool, system_prompt, tools_config, avatar,
		        capabilities_json, custom_skills, tags, source, status, version, machine_id, machine_name, enable_management_tools,
		        last_seen_at, created_at, updated_at
		 FROM agents WHERE id = $1`,
		id,
	).StructScan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	return &a, nil
}

// daemonTaskRetention 是已完成任务在内存中的保留期，用于懒清理，防止无限增长。
// 远大于任务最长生命周期（waitDaemonTask 超时 400s），不会误删在途任务。
const daemonTaskRetention = 10 * time.Minute

// CreateDaemonTask 创建一次等待远端电脑执行的 CLI 任务（内存队列，不落库）。
func (r *AgentRepo) CreateDaemonTask(_ context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error) {
	now := time.Now()
	task := &model.DaemonTask{
		ID:              uuid.NewString(),
		UserID:          userID,
		ConversationID:  conversationID,
		AgentID:         agentID,
		MachineID:       machineID,
		CLITool:         cliTool,
		Prompt:          prompt,
		ContextMessages: contextMessages,
		Status:          "pending",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	r.taskMu.Lock()
	r.sweepDaemonTasksLocked(now)
	r.tasks[task.ID] = task
	r.taskQueue[machineID] = append(r.taskQueue[machineID], task.ID)
	r.taskMu.Unlock()

	r.dispatchMu.RLock()
	dispatcher := r.dispatcher
	r.dispatchMu.RUnlock()
	if dispatcher != nil {
		dispatcher(cloneDaemonTask(task))
	}
	return cloneDaemonTask(task), nil
}

// SetDaemonTaskOrch 关联内存中的 daemon task 到一个编排任务。
func (r *AgentRepo) SetDaemonTaskOrch(_ context.Context, taskID, orchTaskID, workerName string) {
	r.taskMu.Lock()
	defer r.taskMu.Unlock()
	if t, ok := r.tasks[taskID]; ok {
		t.OrchTaskID = orchTaskID
		t.WorkerName = workerName
	}
}

// GetDaemonTaskByOrch 查询属于指定编排任务的内存 daemon task 列表。
func (r *AgentRepo) GetDaemonTasksByOrch(_ context.Context, orchTaskID string) []*model.DaemonTask {
	r.taskMu.Lock()
	defer r.taskMu.Unlock()
	var result []*model.DaemonTask
	for _, t := range r.tasks {
		if t.OrchTaskID == orchTaskID {
			result = append(result, cloneDaemonTask(t))
		}
	}
	return result
}

// GetDaemonTask 按 ID 查询 daemon 任务（内存）。
func (r *AgentRepo) GetDaemonTask(_ context.Context, id string) (*model.DaemonTask, error) {
	r.taskMu.Lock()
	defer r.taskMu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		return nil, nil
	}
	return cloneDaemonTask(task), nil
}

// ClaimDaemonTask 为指定电脑领取一条待执行任务（内存 FIFO，pending→running）。
func (r *AgentRepo) ClaimDaemonTask(_ context.Context, machineID string) (*model.DaemonTask, error) {
	r.taskMu.Lock()
	defer r.taskMu.Unlock()
	queue := r.taskQueue[machineID]
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		task, ok := r.tasks[id]
		if ok && task.Status == "pending" {
			now := time.Now()
			task.Status = "running"
			task.ClaimedAt = &now
			task.UpdatedAt = now
			r.taskQueue[machineID] = queue
			return cloneDaemonTask(task), nil
		}
	}
	r.taskQueue[machineID] = queue
	return nil, nil
}

// CompleteDaemonTask 写入远端 CLI 执行结果（running→completed/failed，内存）。
func (r *AgentRepo) CompleteDaemonTask(_ context.Context, id, machineID, result, taskError string) (bool, error) {
	r.taskMu.Lock()
	defer r.taskMu.Unlock()
	task, ok := r.tasks[id]
	if !ok || task.MachineID != machineID || (task.Status != "running" && task.Status != "pending") {
		return false, nil
	}
	now := time.Now()
	if taskError != "" {
		task.Status = "failed"
		task.Error = taskError
	} else {
		task.Status = "completed"
		task.Result = result
	}
	task.CompletedAt = &now
	task.UpdatedAt = now
	return true, nil
}

// sweepDaemonTasksLocked 清理已完成且超过保留期的任务。调用方须持有 taskMu。
func (r *AgentRepo) sweepDaemonTasksLocked(now time.Time) {
	for id, task := range r.tasks {
		if task.CompletedAt != nil && now.Sub(*task.CompletedAt) > daemonTaskRetention {
			delete(r.tasks, id)
		}
	}
}

// cloneDaemonTask 返回任务副本，避免调用方读到正在被并发修改的内存对象。
func cloneDaemonTask(task *model.DaemonTask) *model.DaemonTask {
	clone := *task
	return &clone
}

// CreateCustom 创建用户自建 Agent
func (r *AgentRepo) CreateCustom(ctx context.Context, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, customSkills string, enableManagementTools bool) (*model.Agent, error) {
	var a model.Agent
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO agents (user_id, name, type, cli_tool, system_prompt, tools_config, avatar, capabilities_json, custom_skills, enable_management_tools, source, status)
		 VALUES (NULLIF($1,'')::uuid, $2, 'custom', $3, $4, $5, $6, $7, $8, $9, 'manual', 'offline')
		 RETURNING id, user_id, name, type, cli_tool, system_prompt, tools_config, avatar,
		           capabilities_json, custom_skills, tags, source, status, version, machine_id, machine_name, enable_management_tools,
		           last_seen_at, created_at, updated_at`,
		userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, customSkills, enableManagementTools,
	).StructScan(&a)
	if err != nil {
		return nil, fmt.Errorf("insert custom agent: %w", err)
	}
	return &a, nil
}

// UpdateCustom 更新用户自建 Agent。userID 为空时跳过归属校验（MCP 模式）。
func (r *AgentRepo) UpdateCustom(ctx context.Context, id, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, customSkills string, enableManagementTools bool) (*model.Agent, error) {
	var a model.Agent
	var args []interface{}
	var query string
	if userID != "" {
		query = `UPDATE agents
		 SET name = $2, cli_tool = $3, system_prompt = $4, tools_config = $5, avatar = $6,
		     capabilities_json = $7, custom_skills = $8, enable_management_tools = $9, updated_at = NOW()
		 WHERE id = $1 AND type = 'custom' AND user_id = $10`
		args = []interface{}{id, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, customSkills, enableManagementTools, userID}
	} else {
		query = `UPDATE agents
		 SET name = $2, cli_tool = $3, system_prompt = $4, tools_config = $5, avatar = $6,
		     capabilities_json = $7, custom_skills = $8, enable_management_tools = $9, updated_at = NOW()
		 WHERE id = $1 AND type = 'custom'`
		args = []interface{}{id, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, customSkills, enableManagementTools}
	}
	query += ` RETURNING id, user_id, name, type, cli_tool, system_prompt, tools_config, avatar,
		           capabilities_json, custom_skills, tags, source, status, version, machine_id, machine_name, enable_management_tools,
		           last_seen_at, created_at, updated_at`
	err := r.db.QueryRowxContext(ctx, query, args...,
	).StructScan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update custom agent: %w", err)
	}
	return &a, nil
}

// UpdateToolsConfig 只更新当前用户可见 Agent 的工具授权字段。
func (r *AgentRepo) UpdateToolsConfig(ctx context.Context, id, userID, toolsConfig string, enableManagementTools bool) (*model.Agent, error) {
	var a model.Agent
	err := r.db.QueryRowxContext(ctx,
		`UPDATE agents
		 SET tools_config = $3, enable_management_tools = $4, updated_at = NOW()
		 WHERE id = $1 AND (user_id = $2 OR user_id IS NULL)
		 RETURNING id, user_id, name, type, cli_tool, system_prompt, tools_config, avatar,
		           capabilities_json, custom_skills, tags, source, status, version, machine_id, machine_name, enable_management_tools,
		           last_seen_at, created_at, updated_at`,
		id, userID, toolsConfig, enableManagementTools,
	).StructScan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update agent tools_config: %w", err)
	}
	return &a, nil
}

// UpdateAgentStatus 更新 Agent 状态
func (r *AgentRepo) UpdateAgentStatus(ctx context.Context, id, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE agents SET status = $2, updated_at = NOW() WHERE id = $1`,
		id, status,
	)
	if err != nil {
		return fmt.Errorf("update agent status: %w", err)
	}
	return nil
}

// UpdateAvatar 仅更新 Agent 头像字段（归属校验由调用方完成）
func (r *AgentRepo) UpdateAvatar(ctx context.Context, id, userID, avatar string) (*model.Agent, error) {
	var a model.Agent
	err := r.db.QueryRowxContext(ctx,
		`UPDATE agents
		 SET avatar = $3, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND type = 'custom'
		 RETURNING id, user_id, name, type, cli_tool, system_prompt, tools_config, avatar,
		           capabilities_json, custom_skills, tags, source, status, version, machine_id, machine_name, enable_management_tools,
		           last_seen_at, created_at, updated_at`,
		id, userID, avatar,
	).StructScan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update agent avatar: %w", err)
	}
	return &a, nil
}

// ClearAgentMachine 清除 Agent 的 machine_id 并设为离线
func (r *AgentRepo) ClearAgentMachine(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE agents SET status = 'offline', machine_id = NULL, updated_at = NOW() WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("clear agent machine: %w", err)
	}
	return nil
}

// MarkMachineAgentsStopped 批量将指定 machine_id 下所有 online 的 Agent 状态设为 stopped。
// 在 daemon WS 断开时调用，防止 Agent 状态在 daemon 离线后仍显示 online。
func (r *AgentRepo) MarkMachineAgentsStopped(ctx context.Context, machineID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE agents SET status = 'stopped', updated_at = NOW() WHERE machine_id = $1 AND status = 'online'`,
		machineID,
	)
	if err != nil {
		return fmt.Errorf("mark machine agents stopped: %w", err)
	}
	return nil
}

// GetAgentsByMachine 查询指定 machine_id 下的所有 Agent
func (r *AgentRepo) GetAgentsByMachine(ctx context.Context, machineID string) ([]model.Agent, error) {
	var list []model.Agent
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, user_id, name, type, cli_tool, system_prompt, tools_config, avatar,
		        capabilities_json, custom_skills, tags, source, status, version, machine_id, machine_name, enable_management_tools,
		        last_seen_at, created_at, updated_at
		 FROM agents WHERE machine_id = $1`,
		machineID,
	)
	if err != nil {
		return nil, fmt.Errorf("get agents by machine: %w", err)
	}
	return list, nil
}

// DeleteOwned 删除 Agent。userID 为空时仅按 ID 删除（MCP 模式）。
func (r *AgentRepo) DeleteOwned(ctx context.Context, id, userID string) (bool, error) {
	var query string
	var args []interface{}
	if userID != "" {
		query = `DELETE FROM agents WHERE id = $1 AND user_id = $2`
		args = []interface{}{id, userID}
	} else {
		query = `DELETE FROM agents WHERE id = $1`
		args = []interface{}{id}
	}
	res, err := r.db.ExecContext(ctx, query, args...,
	)
	if err != nil {
		return false, fmt.Errorf("delete owned agent: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return count > 0, nil
}

// UpdateTags updates the tags field for any agent by ID.
func (r *AgentRepo) UpdateTags(ctx context.Context, id, tags string) (*model.Agent, error) {
	var a model.Agent
	err := r.db.QueryRowxContext(ctx,
		`UPDATE agents SET tags = $2, updated_at = NOW() WHERE id = $1
		 RETURNING id, user_id, name, type, cli_tool, system_prompt, tools_config, avatar,
		           capabilities_json, custom_skills, tags, source, status, version, machine_id, machine_name, enable_management_tools,
		           last_seen_at, created_at, updated_at`,
		id, tags,
	).StructScan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update agent tags: %w", err)
	}
	return &a, nil
}

// UpdateCustomSkills updates the custom_skills field for the current user's custom Agent.
func (r *AgentRepo) UpdateCustomSkills(ctx context.Context, id, userID, customSkills string) (*model.Agent, error) {
	var a model.Agent
	err := r.db.QueryRowxContext(ctx,
		`UPDATE agents SET custom_skills = $3, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND type = 'custom'
		 RETURNING id, user_id, name, type, cli_tool, system_prompt, tools_config, avatar,
		           capabilities_json, custom_skills, tags, source, status, version, machine_id, machine_name, enable_management_tools,
		           last_seen_at, created_at, updated_at`,
		id, userID, customSkills,
	).StructScan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update agent custom_skills: %w", err)
	}
	return &a, nil
}
