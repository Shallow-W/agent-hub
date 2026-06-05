package handler

import (
	"context"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/pkg/ws"
)

// DispatchTask 由任务队列创建回调触发，在线 daemon 立即收到 task.execute。
func (h *DaemonHandler) DispatchTask(task *model.DaemonTask) {
	if task == nil || task.MachineID == "" {
		return
	}

	if !h.daemonHub.IsConnected(task.MachineID) {
		return
	}

	payload := struct {
		Type string            `json:"type"`
		Data *model.DaemonTask `json:"data"`
	}{
		Type: "task.execute",
		Data: task,
	}

	h.daemonHub.RegisterTaskPromise(task.ID)

	if err := h.daemonHub.SendToMachine(task.MachineID, ws.WSMessage{
		Type: "task.execute",
		Data: payload,
	}); err != nil {
		h.daemonHub.RemoveTaskPromise(task.ID)
		h.logger.Warn("daemon task dispatch failed",
			"machine", task.MachineID, "task", task.ID, "error", err)
		h.failTask(task.MachineID, task.ID, "daemon dispatch failed: "+err.Error())
	}
}

// handleTaskComplete 已在 daemon.go 中定义。

func (h *DaemonHandler) failTask(machineID, taskID, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	machine := &model.DaemonMachine{ID: machineID}
	if err := h.agentSvc.CompleteDaemonTask(ctx, machine, taskID, "", reason); err != nil {
		h.logger.Warn("mark daemon task failed", "machine", machineID, "task", taskID, "error", err)
	}

	// 清理 promise
	h.daemonHub.ResolveTask(taskID, &ws.TaskResult{
		TaskID: taskID,
		Error:  reason,
	})
}
