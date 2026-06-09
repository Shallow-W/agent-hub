package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/agent-hub/backend/internal/model"
)

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

// UpdateMachineAPIKey 更新电脑的 API Key 哈希（用于重新生成连接命令）。
func (r *AgentRepo) UpdateMachineAPIKey(ctx context.Context, id, apiKeyHash string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE daemon_machines SET api_key_hash = $2, updated_at = NOW() WHERE id = $1`,
		id, apiKeyHash,
	)
	if err != nil {
		return fmt.Errorf("update machine api key: %w", err)
	}
	return nil
}

// GetDaemonMachineByID 按 ID 查询电脑连接
func (r *AgentRepo) GetDaemonMachineByID(ctx context.Context, id string) (*model.DaemonMachine, error) {
	var m model.DaemonMachine
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, user_id, name, api_key_hash, machine_id, status,
		        last_seen_at, created_at, updated_at
		 FROM daemon_machines WHERE id = $1`,
		id,
	).StructScan(&m)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get daemon machine by id: %w", err)
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

// SetMachineAndAgentsOnline 标记机器及其 Agent 为在线（仅状态变更时调用）。
func (r *AgentRepo) SetMachineAndAgentsOnline(ctx context.Context, machineID string) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE daemon_machines SET status = 'connected', last_seen_at = NOW(), updated_at = NOW() WHERE id = $1`,
		machineID,
	); err != nil {
		return fmt.Errorf("set machine online: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE agents SET status = 'online', last_seen_at = NOW(), updated_at = NOW() WHERE machine_id = $1`,
		machineID,
	); err != nil {
		return fmt.Errorf("set agents online: %w", err)
	}
	return nil
}

// SetMachineAndAgentsOffline 标记机器及其 Agent 为离线（仅状态变更时调用）。
// machineID 为空时标记全部机器和 Agent 为离线（服务启动时使用）。
func (r *AgentRepo) SetMachineAndAgentsOffline(ctx context.Context, machineID string) error {
	if machineID == "" {
		if _, err := r.db.ExecContext(ctx,
			`UPDATE daemon_machines SET status = 'offline', updated_at = NOW() WHERE status = 'connected'`,
		); err != nil {
			return fmt.Errorf("set all machines offline: %w", err)
		}
		if _, err := r.db.ExecContext(ctx,
			`UPDATE agents SET status = 'offline', updated_at = NOW() WHERE status = 'online'`,
		); err != nil {
			return fmt.Errorf("set all agents offline: %w", err)
		}
		return nil
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE daemon_machines SET status = 'offline', updated_at = NOW() WHERE id = $1`,
		machineID,
	); err != nil {
		return fmt.Errorf("set machine offline: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE agents SET status = 'offline', updated_at = NOW() WHERE machine_id = $1`,
		machineID,
	); err != nil {
		return fmt.Errorf("set agents offline: %w", err)
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
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert machine agent candidate: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO daemon_agent_candidates (machine_id, name, cli_tool, version, capabilities_json, last_seen_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (machine_id, cli_tool) DO UPDATE
		 SET name = EXCLUDED.name,
		     version = EXCLUDED.version,
		     capabilities_json = EXCLUDED.capabilities_json,
		     last_seen_at = NOW(),
		     updated_at = NOW()`,
		machineID, name, cliTool, version, capabilitiesJSON,
	); err != nil {
		return fmt.Errorf("upsert machine agent candidate: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE agents
		 SET capabilities_json = $3,
		     version = $4,
		     status = 'online',
		     last_seen_at = NOW(),
		     updated_at = NOW()
		 WHERE machine_id = $1 AND cli_tool = $2 AND source = 'daemon'`,
		machineID, cliTool, capabilitiesJSON, version,
	); err != nil {
		return fmt.Errorf("sync machine agent capabilities: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert machine agent candidate: %w", err)
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
func (r *AgentRepo) AddCandidateAgent(ctx context.Context, userID, candidateID, displayName, expectedCLITool, systemPrompt, toolsConfig, customSkills string) (*model.Agent, error) {
	var a model.Agent
	err := r.db.QueryRowxContext(ctx,
		`WITH candidate AS (
		     SELECT c.*, m.user_id, m.name AS machine_name
		     FROM daemon_agent_candidates c
		     JOIN daemon_machines m ON m.id = c.machine_id
		     WHERE c.id = $1 AND m.user_id = $2 AND c.cli_tool = $5
		 )
		 INSERT INTO agents (user_id, name, type, cli_tool, system_prompt, tools_config, capabilities_json, custom_skills, source, status, version, machine_id, machine_name, last_seen_at)
		 SELECT user_id, $3, 'custom', cli_tool, $4, $6, capabilities_json, $7, 'daemon', 'online', version, machine_id, machine_name, NOW()
		 FROM candidate
		 RETURNING id, user_id, name, type, cli_tool, system_prompt, tools_config, avatar,
		           capabilities_json, custom_skills, tags, source, status, version, machine_id, machine_name, enable_management_tools, last_seen_at,
		           created_at, updated_at`,
		candidateID, userID, displayName, systemPrompt, expectedCLITool, toolsConfig, customSkills,
	).StructScan(&a)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("add candidate agent: %w", err)
	}
	return &a, nil
}
