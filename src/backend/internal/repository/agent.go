package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
	"github.com/jmoiron/sqlx"
)

// AgentRepo Agent 数据访问
type AgentRepo struct {
	db *sqlx.DB
}

// NewAgentRepo 创建 Agent 仓库
func NewAgentRepo(db *sqlx.DB) *AgentRepo {
	return &AgentRepo{db: db}
}

// ListAvailable 查询系统 Agent 和当前用户自建 Agent
func (r *AgentRepo) ListAvailable(ctx context.Context, userID string) ([]model.Agent, error) {
	list := make([]model.Agent, 0)
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, user_id, name, type, cli_tool, system_prompt, avatar,
		        capabilities_json, source, status, version, machine_id, machine_name, last_seen_at,
		        created_at, updated_at
		 FROM agents
		 WHERE user_id IS NULL OR user_id = $1
		 ORDER BY type ASC, updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	return list, nil
}

// CreateDaemonMachine 创建一台等待连接的电脑
func (r *AgentRepo) CreateDaemonMachine(ctx context.Context, userID, name, apiKeyHash string) (*model.DaemonMachine, error) {
	var m model.DaemonMachine
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO daemon_machines (user_id, name, api_key_hash)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, name, api_key_hash, machine_id, status,
		           last_seen_at, created_at, updated_at`,
		userID, name, apiKeyHash,
	).StructScan(&m)
	if err != nil {
		return nil, fmt.Errorf("insert daemon machine: %w", err)
	}
	return &m, nil
}

// ListDaemonMachines 查询用户已创建的电脑连接
func (r *AgentRepo) ListDaemonMachines(ctx context.Context, userID string) ([]model.DaemonMachine, error) {
	list := make([]model.DaemonMachine, 0)
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, user_id, name, api_key_hash, machine_id, status,
		        last_seen_at, created_at, updated_at
		 FROM daemon_machines
		 WHERE user_id = $1
		 ORDER BY updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list daemon machines: %w", err)
	}
	return list, nil
}

// DeleteDaemonMachine 删除当前用户创建的电脑连接。
func (r *AgentRepo) DeleteDaemonMachine(ctx context.Context, id, userID string) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin delete daemon machine: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM agents WHERE machine_id = $1 AND user_id = $2`,
		id, userID,
	); err != nil {
		return false, fmt.Errorf("delete machine agents: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`DELETE FROM daemon_machines WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return false, fmt.Errorf("delete daemon machine: %w", err)
	}

	count, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	if count == 0 {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit delete daemon machine: %w", err)
	}
	return count > 0, nil
}

// GetDaemonMachineByAPIKeyHash 按机器 API Key 哈希查找电脑
func (r *AgentRepo) GetDaemonMachineByAPIKeyHash(ctx context.Context, apiKeyHash string) (*model.DaemonMachine, error) {
	var m model.DaemonMachine
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, user_id, name, api_key_hash, machine_id, status,
		        last_seen_at, created_at, updated_at
		 FROM daemon_machines WHERE api_key_hash = $1`,
		apiKeyHash,
	).StructScan(&m)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get daemon machine by api key: %w", err)
	}
	return &m, nil
}

// MarkDaemonMachineConnected 标记电脑在线
func (r *AgentRepo) MarkDaemonMachineConnected(ctx context.Context, id, machineID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE daemon_machines
		 SET machine_id = $2, status = 'connected', last_seen_at = NOW(), updated_at = NOW()
		 WHERE id = $1`,
		id, machineID,
	)
	if err != nil {
		return fmt.Errorf("mark daemon machine connected: %w", err)
	}
	return nil
}

// UpsertSystemAgent 写入 daemon 上报的系统 Agent
func (r *AgentRepo) UpsertSystemAgent(ctx context.Context, name, cliTool, version, capabilitiesJSON string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO agents (name, type, cli_tool, capabilities_json, source, status, version, last_seen_at)
		 VALUES ($1, 'system', $2, $3, 'daemon', 'online', $4, NOW())
		 ON CONFLICT (cli_tool) WHERE user_id IS NULL DO UPDATE
		 SET name = EXCLUDED.name,
		     capabilities_json = EXCLUDED.capabilities_json,
		     source = 'daemon',
		     status = 'online',
		     version = EXCLUDED.version,
		     last_seen_at = NOW(),
		     updated_at = NOW()`,
		name, cliTool, capabilitiesJSON, version,
	)
	if err != nil {
		return fmt.Errorf("upsert system agent: %w", err)
	}
	return nil
}

// UpsertMachineAgent 写入指定电脑上报的 Agent
func (r *AgentRepo) UpsertMachineAgent(ctx context.Context, userID, machineID, machineName, name, cliTool, version, capabilitiesJSON string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO agents (user_id, name, type, cli_tool, capabilities_json, source, status, version, machine_id, machine_name, last_seen_at)
		 VALUES ($1, $4, 'system', $5, $7, 'daemon', 'online', $6, $2, $3, NOW())`,
		userID, machineID, machineName, name, cliTool, version, capabilitiesJSON,
	)
	if err != nil {
		return fmt.Errorf("upsert machine agent: %w", err)
	}
	return nil
}

// UpsertMachineAgentCandidate 保存指定电脑扫描到的候选 Agent。
func (r *AgentRepo) UpsertMachineAgentCandidate(ctx context.Context, machineID, name, cliTool, version, capabilitiesJSON string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO daemon_agent_candidates (machine_id, name, cli_tool, version, capabilities_json, last_seen_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (machine_id, cli_tool) DO UPDATE
		 SET name = EXCLUDED.name,
		     version = EXCLUDED.version,
		     capabilities_json = EXCLUDED.capabilities_json,
		     last_seen_at = NOW(),
		     updated_at = NOW()`,
		machineID, name, cliTool, version, capabilitiesJSON,
	)
	if err != nil {
		return fmt.Errorf("upsert machine agent candidate: %w", err)
	}
	return nil
}

// ListAgentCandidates 查询当前用户电脑上检测到的候选 Agent。
func (r *AgentRepo) ListAgentCandidates(ctx context.Context, userID string) ([]model.AgentCandidate, error) {
	list := make([]model.AgentCandidate, 0)
	err := r.db.SelectContext(ctx, &list,
		`SELECT c.id, c.machine_id, m.name AS machine_name, c.name, c.cli_tool,
		        c.version, c.capabilities_json, c.last_seen_at, c.created_at, c.updated_at
		 FROM daemon_agent_candidates c
		 JOIN daemon_machines m ON m.id = c.machine_id
		 WHERE m.user_id = $1
		 ORDER BY m.updated_at DESC, c.updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list agent candidates: %w", err)
	}
	return list, nil
}

// AddCandidateAgent 将候选 Agent 添加到当前用户的可用 Agent 列表。
func (r *AgentRepo) AddCandidateAgent(ctx context.Context, userID, candidateID, displayName, systemPrompt string) (*model.Agent, error) {
	var a model.Agent
	err := r.db.QueryRowxContext(ctx,
		`WITH candidate AS (
		     SELECT c.*, m.user_id, m.name AS machine_name
		     FROM daemon_agent_candidates c
		     JOIN daemon_machines m ON m.id = c.machine_id
		     WHERE c.id = $1 AND m.user_id = $2
		 )
		 INSERT INTO agents (user_id, name, type, cli_tool, system_prompt, capabilities_json, source, status, version, machine_id, machine_name, last_seen_at)
		 SELECT user_id, $3, 'custom', cli_tool, $4, capabilities_json, 'daemon', 'online', version, machine_id, machine_name, NOW()
		 FROM candidate
		 RETURNING id, user_id, name, type, cli_tool, system_prompt, avatar,
		           capabilities_json, source, status, version, machine_id, machine_name, last_seen_at,
		           created_at, updated_at`,
		candidateID, userID, displayName, systemPrompt,
	).StructScan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("add candidate agent: %w", err)
	}
	return &a, nil
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
		`SELECT id, user_id, name, type, cli_tool, system_prompt, avatar,
		        capabilities_json, source, status, version, machine_id, machine_name, last_seen_at,
		        created_at, updated_at
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

// CreateDaemonTask 创建一次等待远端电脑执行的 CLI 任务。
func (r *AgentRepo) CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt string) (*model.DaemonTask, error) {
	var t model.DaemonTask
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO daemon_tasks (user_id, conversation_id, agent_id, machine_id, cli_tool, prompt)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, user_id, conversation_id, agent_id, machine_id, cli_tool, prompt,
		           status, result, error, claimed_at, completed_at, created_at, updated_at`,
		userID, conversationID, agentID, machineID, cliTool, prompt,
	).StructScan(&t)
	if err != nil {
		return nil, fmt.Errorf("insert daemon task: %w", err)
	}
	return &t, nil
}

// GetDaemonTask 按 ID 查询 daemon 任务。
func (r *AgentRepo) GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error) {
	var t model.DaemonTask
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, user_id, conversation_id, agent_id, machine_id, cli_tool, prompt,
		        status, result, error, claimed_at, completed_at, created_at, updated_at
		 FROM daemon_tasks WHERE id = $1`,
		id,
	).StructScan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get daemon task: %w", err)
	}
	return &t, nil
}

// ClaimDaemonTask 为指定电脑领取一条待执行任务。
func (r *AgentRepo) ClaimDaemonTask(ctx context.Context, machineID string) (*model.DaemonTask, error) {
	var t model.DaemonTask
	err := r.db.QueryRowxContext(ctx,
		`WITH next_task AS (
		     SELECT id
		     FROM daemon_tasks
		     WHERE machine_id = $1 AND status = 'pending'
		     ORDER BY created_at ASC
		     LIMIT 1
		     FOR UPDATE SKIP LOCKED
		 )
		 UPDATE daemon_tasks
		 SET status = 'running', claimed_at = NOW(), updated_at = NOW()
		 WHERE id = (SELECT id FROM next_task)
		 RETURNING id, user_id, conversation_id, agent_id, machine_id, cli_tool, prompt,
		           status, result, error, claimed_at, completed_at, created_at, updated_at`,
		machineID,
	).StructScan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim daemon task: %w", err)
	}
	return &t, nil
}

// CompleteDaemonTask 写入远端 CLI 执行结果。
func (r *AgentRepo) CompleteDaemonTask(ctx context.Context, id, machineID, result, taskError string) (bool, error) {
	status := "completed"
	if taskError != "" {
		status = "failed"
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE daemon_tasks
		 SET status = $3, result = $4, error = $5, completed_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND machine_id = $2 AND status = 'running'`,
		id, machineID, status, result, taskError,
	)
	if err != nil {
		return false, fmt.Errorf("complete daemon task: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return count > 0, nil
}

// CreateCustom 创建用户自建 Agent
func (r *AgentRepo) CreateCustom(ctx context.Context, userID, name, cliTool, systemPrompt, avatar, capabilitiesJSON string) (*model.Agent, error) {
	var a model.Agent
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO agents (user_id, name, type, cli_tool, system_prompt, avatar, capabilities_json, source, status)
		 VALUES ($1, $2, 'custom', $3, $4, $5, $6, 'manual', 'offline')
		 RETURNING id, user_id, name, type, cli_tool, system_prompt, avatar,
		           capabilities_json, source, status, version, machine_id, machine_name, last_seen_at,
		           created_at, updated_at`,
		userID, name, cliTool, systemPrompt, avatar, capabilitiesJSON,
	).StructScan(&a)
	if err != nil {
		return nil, fmt.Errorf("insert custom agent: %w", err)
	}
	return &a, nil
}

// UpdateCustom 更新用户自建 Agent
func (r *AgentRepo) UpdateCustom(ctx context.Context, id, userID, name, cliTool, systemPrompt, avatar, capabilitiesJSON string) (*model.Agent, error) {
	var a model.Agent
	err := r.db.QueryRowxContext(ctx,
		`UPDATE agents
		 SET name = $3, cli_tool = $4, system_prompt = $5, avatar = $6,
		     capabilities_json = $7, updated_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND type = 'custom'
		 RETURNING id, user_id, name, type, cli_tool, system_prompt, avatar,
		           capabilities_json, source, status, version, machine_id, machine_name, last_seen_at,
		           created_at, updated_at`,
		id, userID, name, cliTool, systemPrompt, avatar, capabilitiesJSON,
	).StructScan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update custom agent: %w", err)
	}
	return &a, nil
}

// DeleteOwned 删除当前用户拥有的 Agent，包括自建 Agent 和电脑上报的系统 Agent。
func (r *AgentRepo) DeleteOwned(ctx context.Context, id, userID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM agents WHERE id = $1 AND user_id = $2`,
		id, userID,
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
