package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"nhooyr.io/websocket"
)

type daemonConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *daemonConn) write(ctx context.Context, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Write(ctx, websocket.MessageText, data)
}

func (h *DaemonHandler) registerDaemonConn(machineID string, conn *daemonConn) {
	h.connMu.Lock()
	h.conns[machineID] = conn
	h.connMu.Unlock()
}

func (h *DaemonHandler) unregisterDaemonConn(machineID string, conn *daemonConn) {
	h.connMu.Lock()
	if h.conns[machineID] == conn {
		delete(h.conns, machineID)
	}
	h.connMu.Unlock()
}

func (h *DaemonHandler) daemonConn(machineID string) *daemonConn {
	h.connMu.RLock()
	defer h.connMu.RUnlock()
	return h.conns[machineID]
}

// DispatchTask 由任务队列创建回调触发，在线 daemon 立即收到 task.execute。
func (h *DaemonHandler) DispatchTask(task *model.DaemonTask) {
	if task == nil || task.MachineID == "" {
		return
	}
	h.startDaemonDispatch(task.MachineID)
}

func (h *DaemonHandler) startDaemonDispatch(machineID string) {
	h.dispatchMu.Lock()
	if h.dispatching[machineID] {
		h.dispatchAgain[machineID] = true
		h.dispatchMu.Unlock()
		return
	}
	h.dispatching[machineID] = true
	h.dispatchMu.Unlock()

	go func() {
		for {
			h.dispatchPendingTasks(machineID)
			h.dispatchMu.Lock()
			if h.dispatchAgain[machineID] {
				delete(h.dispatchAgain, machineID)
				h.dispatchMu.Unlock()
				continue
			}
			delete(h.dispatching, machineID)
			h.dispatchMu.Unlock()
			return
		}
	}()
}

func (h *DaemonHandler) dispatchPendingTasks(machineID string) {
	for {
		conn := h.daemonConn(machineID)
		if conn == nil {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		task, err := h.agentSvc.ClaimDaemonTask(ctx, &model.DaemonMachine{ID: machineID})
		cancel()
		if err != nil {
			h.logger.Error("claim daemon task for ws dispatch failed", "machine", machineID, "error", err)
			return
		}
		if task == nil {
			return
		}
		if err := h.sendDaemonTask(conn, task); err != nil {
			h.logger.Warn("daemon task ws dispatch failed", "machine", machineID, "task", task.ID, "error", err)
			return
		}
	}
}

func (h *DaemonHandler) sendDaemonTask(conn *daemonConn, task *model.DaemonTask) error {
	payload := struct {
		Type string            `json:"type"`
		Data *model.DaemonTask `json:"data"`
	}{
		Type: "task.execute",
		Data: task,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return conn.write(ctx, data)
}

func (h *DaemonHandler) handleTaskDone(ctx context.Context, data json.RawMessage, machine *model.DaemonMachine, forceError bool) {
	if machine == nil {
		h.logger.Warn("daemon task result without machine")
		return
	}
	var req struct {
		TaskID string `json:"task_id"`
		Result string `json:"result"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		h.logger.Warn("invalid daemon task result", "error", err)
		return
	}
	if forceError && req.Error == "" {
		req.Error = "daemon task failed"
	}
	taskCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := h.agentSvc.CompleteDaemonTask(taskCtx, machine, req.TaskID, req.Result, req.Error); err != nil {
		h.logger.Error("complete daemon task from ws failed", slog.String("machine", machine.ID), slog.String("task", req.TaskID), "error", err)
		return
	}
	h.agentSvc.TouchMachine(machine.ID)
	h.startDaemonDispatch(machine.ID)
}

func (h *DaemonHandler) handleTaskHeartbeat(data json.RawMessage, machine *model.DaemonMachine) {
	if machine == nil {
		return
	}
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		h.logger.Warn("invalid daemon task heartbeat", "error", err)
		return
	}
	h.agentSvc.TouchMachine(machine.ID)
}
