package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type OrchTaskRepo struct {
	db *sqlx.DB
}

const insertOrchTaskWithReplySQL = `INSERT INTO orch_tasks (id, conversation_id, user_id, orch_agent_id, status, dispatch_plan, worker_status, worker_results, summary, original_message, source_message_id, dispatch_message_id, kb_preload, round, round_history, created_at, updated_at)
	 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, '')::uuid, NULLIF($12, '')::uuid, $13, $14, $15, $16, $17)`

const insertOrchTaskWithoutReplySQL = `INSERT INTO orch_tasks (id, conversation_id, user_id, orch_agent_id, status, dispatch_plan, worker_status, worker_results, summary, original_message, kb_preload, round, round_history, created_at, updated_at)
	 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

func NewOrchTaskRepo(db *sqlx.DB) *OrchTaskRepo {
	return &OrchTaskRepo{db: db}
}

func (r *OrchTaskRepo) Create(ctx context.Context, task *model.OrchTask) error {
	if task.ID == "" {
		task.ID = uuid.NewString()
	}
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now
	err := r.insert(ctx, task, true, task.DispatchPlan)
	if err == nil {
		return nil
	}
	if isInvalidJSONText(err) {
		err = r.insert(ctx, task, true, legacyJSONBDispatchPlan(task.DispatchPlan))
		if err == nil {
			return nil
		}
	}
	if !shouldRetryWithoutReplyColumns(err) {
		return fmt.Errorf("insert orch task: %w", err)
	}

	err = r.insert(ctx, task, false, task.DispatchPlan)
	if err == nil {
		return nil
	}
	if isInvalidJSONText(err) {
		err = r.insert(ctx, task, false, legacyJSONBDispatchPlan(task.DispatchPlan))
		if err == nil {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("insert orch task: %w", err)
	}
	return nil
}

func (r *OrchTaskRepo) insert(ctx context.Context, task *model.OrchTask, includeReplyColumns bool, dispatchPlan string) error {
	if includeReplyColumns {
		_, err := r.db.ExecContext(ctx,
			insertOrchTaskWithReplySQL,
			task.ID, task.ConversationID, task.UserID, task.OrchAgentID, task.Status,
			dispatchPlan, task.WorkerStatus, task.WorkerResults, task.Summary, task.OriginalMessage,
			task.SourceMessageID, task.DispatchMessageID, task.KBPreload,
			task.Round, task.RoundHistory,
			task.CreatedAt, task.UpdatedAt,
		)
		return err
	}
	_, err := r.db.ExecContext(ctx,
		insertOrchTaskWithoutReplySQL,
		task.ID, task.ConversationID, task.UserID, task.OrchAgentID, task.Status,
		dispatchPlan, task.WorkerStatus, task.WorkerResults, task.Summary, task.OriginalMessage, task.KBPreload,
		task.Round, task.RoundHistory,
		task.CreatedAt, task.UpdatedAt,
	)
	return err
}

func (r *OrchTaskRepo) GetByID(ctx context.Context, id string) (*model.OrchTask, error) {
	var t model.OrchTask
	err := r.db.QueryRowxContext(ctx,
		`SELECT id, conversation_id, user_id, orch_agent_id, status, dispatch_plan, worker_status, worker_results, summary, original_message, COALESCE(source_message_id::text, '') AS source_message_id, COALESCE(dispatch_message_id::text, '') AS dispatch_message_id, kb_preload, round, round_history, created_at, updated_at
		 FROM orch_tasks WHERE id = $1`, id,
	).StructScan(&t)
	if isUndefinedColumn(err) {
		err = r.db.QueryRowxContext(ctx,
			`SELECT id, conversation_id, user_id, orch_agent_id, status, dispatch_plan, worker_status, worker_results, summary, original_message, kb_preload, round, round_history, created_at, updated_at
			 FROM orch_tasks WHERE id = $1`, id,
		).StructScan(&t)
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get orch task: %w", err)
	}
	return &t, nil
}

func (r *OrchTaskRepo) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE orch_tasks SET status = $2, updated_at = NOW() WHERE id = $1`,
		id, status,
	)
	if err != nil {
		return fmt.Errorf("update orch task status: %w", err)
	}
	return nil
}

func (r *OrchTaskRepo) UpdateDispatchMessageID(ctx context.Context, id, messageID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE orch_tasks SET dispatch_message_id = NULLIF($2, '')::uuid, updated_at = NOW() WHERE id = $1`,
		id, messageID,
	)
	if isUndefinedColumn(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("update orch task dispatch message: %w", err)
	}
	return nil
}

func isUndefinedColumn(err error) bool {
	if err == nil {
		return false
	}
	var sqlState interface{ SQLState() string }
	if errors.As(err, &sqlState) {
		return sqlState.SQLState() == "42703"
	}
	return false
}

func isInvalidJSONText(err error) bool {
	if err == nil {
		return false
	}
	var sqlState interface{ SQLState() string }
	if errors.As(err, &sqlState) {
		return sqlState.SQLState() == "22P02" && strings.Contains(strings.ToLower(err.Error()), "json")
	}
	return false
}

func shouldRetryWithoutReplyColumns(err error) bool {
	if isUndefinedColumn(err) || isReplyReferenceInsertError(err) {
		return true
	}
	return false
}

func isReplyReferenceInsertError(err error) bool {
	if err == nil {
		return false
	}
	var sqlState interface{ SQLState() string }
	if !errors.As(err, &sqlState) {
		return false
	}
	switch sqlState.SQLState() {
	case "23503":
		// source_message_id / dispatch_message_id are optional reply anchors.
		// If either reference is unavailable, keep the lifecycle record and
		// lose only the reply anchor instead of dropping worker dispatch.
		return true
	case "42804":
		// Older or partially migrated databases can disagree on the reply anchor
		// column types. Keep the orchestration lifecycle even if anchors cannot
		// be stored.
		return true
	case "22P02":
		// Invalid JSON is handled separately for dispatch_plan. Other invalid
		// text representation errors here are usually optional UUID anchors.
		return !strings.Contains(strings.ToLower(err.Error()), "json")
	default:
		return false
	}
}

func legacyJSONBDispatchPlan(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	return string(encoded)
}

// UpdateWorkerResult 更新单个 worker 的状态和结果，返回 true 表示所有 worker 都已完成。
// Uses explicit transaction to ensure SELECT FOR UPDATE + UPDATE atomicity.
func (r *OrchTaskRepo) UpdateWorkerResult(ctx context.Context, id, workerName, status, result string) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var t model.OrchTask
	err = tx.QueryRowxContext(ctx,
		`SELECT worker_status, worker_results FROM orch_tasks WHERE id = $1 FOR UPDATE`, id,
	).StructScan(&t)
	if err != nil {
		return false, fmt.Errorf("lock orch task: %w", err)
	}

	var wsMap map[string]string
	if t.WorkerStatus != "" {
		_ = json.Unmarshal([]byte(t.WorkerStatus), &wsMap)
	}
	if wsMap == nil {
		wsMap = make(map[string]string)
	}
	wsMap[workerName] = status

	var wrMap map[string]string
	if t.WorkerResults != "" {
		_ = json.Unmarshal([]byte(t.WorkerResults), &wrMap)
	}
	if wrMap == nil {
		wrMap = make(map[string]string)
	}
	if result != "" {
		wrMap[workerName] = result
	}

	wsJSON, _ := json.Marshal(wsMap)
	wrJSON, _ := json.Marshal(wrMap)

	_, err = tx.ExecContext(ctx,
		`UPDATE orch_tasks SET worker_status = $2, worker_results = $3, updated_at = NOW() WHERE id = $1`,
		id, string(wsJSON), string(wrJSON),
	)
	if err != nil {
		return false, fmt.Errorf("update orch worker result: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	allDone := true
	for _, s := range wsMap {
		if s != "completed" && s != "failed" {
			allDone = false
			break
		}
	}
	return allDone, nil
}

// SetSummaryAndEvaluate saves the summary and transitions status to evaluating (instead of completed).
func (r *OrchTaskRepo) SetSummaryAndEvaluate(ctx context.Context, id, summary string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE orch_tasks SET summary = $2, status = $3, updated_at = NOW() WHERE id = $1`,
		id, summary, model.OrchTaskEvaluating,
	)
	if err != nil {
		return fmt.Errorf("set orch summary evaluate: %w", err)
	}
	return nil
}

// IncrementRound archives current round results into round_history, resets worker state,
// increments round counter, and transitions status back to workers_running.
func (r *OrchTaskRepo) IncrementRound(ctx context.Context, id string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var currentRound int
	var currentStatus string
	var historyJSON, wsJSON, wrJSON string
	err = tx.QueryRowxContext(ctx,
		`SELECT round, status, COALESCE(round_history::text, '[]'), COALESCE(worker_status::text, '{}'), COALESCE(worker_results::text, '{}') FROM orch_tasks WHERE id = $1 FOR UPDATE`, id,
	).Scan(&currentRound, &currentStatus, &historyJSON, &wsJSON, &wrJSON)
	if err != nil {
		return fmt.Errorf("lock orch task for round increment: %w", err)
	}

	// Verify we are transitioning from a valid evaluation/redispatch state.
	if currentStatus != model.OrchTaskCompleted && currentStatus != model.OrchTaskEvaluating && currentStatus != model.OrchTaskWorkersRunning {
		return fmt.Errorf("increment round: unexpected status %q for orch task %s", currentStatus, id)
	}

	var history []json.RawMessage
	_ = json.Unmarshal([]byte(historyJSON), &history)

	entry := map[string]interface{}{
		"round":          currentRound,
		"worker_status":  json.RawMessage(wsJSON),
		"worker_results": json.RawMessage(wrJSON),
	}
	entryBytes, _ := json.Marshal(entry)
	history = append(history, entryBytes)
	newHistory, _ := json.Marshal(history)

	_, err = tx.ExecContext(ctx,
		`UPDATE orch_tasks SET round = round + 1, worker_status = '{}', worker_results = '{}',
		 round_history = $2, status = $3, summary = '', updated_at = NOW() WHERE id = $1`,
		id, string(newHistory), model.OrchTaskWorkersRunning,
	)
	if err != nil {
		return fmt.Errorf("increment round: %w", err)
	}

	return tx.Commit()
}

// UpdateStatusCAS atomically transitions status only if current status matches fromStatus.
// Returns true if the transition succeeded.
func (r *OrchTaskRepo) UpdateStatusCAS(ctx context.Context, id, fromStatus, toStatus string) (bool, error) {
	var err error
	if fromStatus == "" {
		_, err = r.db.ExecContext(ctx,
			`UPDATE orch_tasks SET status = $2, updated_at = NOW() WHERE id = $1`,
			id, toStatus,
		)
	} else {
		result, execErr := r.db.ExecContext(ctx,
			`UPDATE orch_tasks SET status = $3, updated_at = NOW() WHERE id = $1 AND status = $2`,
			id, fromStatus, toStatus,
		)
		if execErr != nil {
			return false, fmt.Errorf("cas update orch task status: %w", execErr)
		}
		rows, _ := result.RowsAffected()
		return rows > 0, nil
	}
	if err != nil {
		return false, fmt.Errorf("update orch task status: %w", err)
	}
	return true, nil
}
