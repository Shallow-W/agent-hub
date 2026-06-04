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
	h.failInFlightTask(machineID, "daemon websocket disconnected")
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
	if h.dispatching[machineID] || h.inFlight[machineID] != "" {
		h.dispatchMu.Unlock()
		return
	}
	h.dispatching[machineID] = true
	h.dispatchMu.Unlock()

	go func() {
		h.dispatchNextTask(machineID)
		h.dispatchMu.Lock()
		delete(h.dispatching, machineID)
		h.dispatchMu.Unlock()
	}()
}

func (h *DaemonHandler) dispatchNextTask(machineID string) {
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
	h.setInFlightTask(machineID, task.ID)
	if err := h.sendDaemonTask(conn, task); err != nil {
		h.clearInFlightTask(machineID, task.ID)
		h.logger.Warn("daemon task ws dispatch failed", "machine", machineID, "task", task.ID, "error", err)
		h.failTask(machineID, task.ID, "daemon websocket dispatch failed: "+err.Error())
		conn.conn.Close(websocket.StatusInternalError, "daemon task dispatch failed")
		return
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
		h.clearInFlightTask(machine.ID, req.TaskID)
		h.logger.Error("complete daemon task from ws failed", slog.String("machine", machine.ID), slog.String("task", req.TaskID), "error", err)
		h.startDaemonDispatch(machine.ID)
		return
	}
	h.clearInFlightTask(machine.ID, req.TaskID)
	h.agentSvc.TouchMachine(machine.ID)
	h.startDaemonDispatch(machine.ID)
}

func (h *DaemonHandler) clearInFlightTask(machineID, taskID string) {
	h.dispatchMu.Lock()
	if h.inFlight[machineID] == taskID || taskID == "" {
		delete(h.inFlight, machineID)
	}
	h.dispatchMu.Unlock()
}

func (h *DaemonHandler) setInFlightTask(machineID, taskID string) {
	h.dispatchMu.Lock()
	h.inFlight[machineID] = taskID
	h.dispatchMu.Unlock()
}

func (h *DaemonHandler) failInFlightTask(machineID, reason string) {
	h.dispatchMu.Lock()
	taskID := h.inFlight[machineID]
	delete(h.inFlight, machineID)
	h.dispatchMu.Unlock()
	if taskID != "" {
		h.failTask(machineID, taskID, reason)
	}
}

func (h *DaemonHandler) failTask(machineID, taskID, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	machine := &model.DaemonMachine{ID: machineID}
	if err := h.agentSvc.CompleteDaemonTask(ctx, machine, taskID, "", reason); err != nil {
		h.logger.Warn("mark daemon task failed", "machine", machineID, "task", taskID, "error", err)
	}
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
