package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/service"
	"github.com/agent-hub/backend/pkg/ws"
	"github.com/gin-gonic/gin"
	"nhooyr.io/websocket"
)

// DaemonHandler 处理本地守护进程连接
type DaemonHandler struct {
	agentSvc       *service.AgentService
	orchSvc        *service.OrchestratorService
	token          string
	logger         *slog.Logger
	allowedOrigins []string
	daemonHub      *ws.DaemonHub
	userHub        *ws.Hub
}

// NewDaemonHandler 创建 daemon WebSocket 处理器
func NewDaemonHandler(agentSvc *service.AgentService, orchSvc *service.OrchestratorService, token string, logger *slog.Logger, allowedOrigins []string, daemonHub *ws.DaemonHub, userHub *ws.Hub) *DaemonHandler {
	return &DaemonHandler{
		agentSvc:       agentSvc,
		orchSvc:        orchSvc,
		token:          token,
		logger:         logger,
		allowedOrigins: allowedOrigins,
		daemonHub:      daemonHub,
		userHub:        userHub,
	}
}

// WithMachine is a higher-order function that authenticates a daemon machine via
// query token and passes the authenticated machine to the wrapped handler.
// If authentication fails, it responds with 401 and does not call fn.
func (h *DaemonHandler) WithMachine(fn func(*gin.Context, *model.DaemonMachine)) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("token")
		machine, err := h.authenticateMachine(c.Request.Context(), token)
		if err != nil || machine == nil {
			middleware.ErrorResponse(c, http.StatusUnauthorized, 40100, "认证失败")
			return
		}
		fn(c, machine)
	}
}

// Handle 处理 daemon WebSocket 连接
func (h *DaemonHandler) Handle(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		token = c.Query("key")
	}
	machine, err := h.authenticateMachine(c.Request.Context(), token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40120, "message": "无效 daemon token", "data": nil})
		return
	}

	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		OriginPatterns: h.allowedOrigins,
	})
	if err != nil {
		h.logger.Error("daemon websocket accept failed", "error", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "disconnect")

	// Daemon registration can include hundreds of skills with full content (multi-MB).
	conn.SetReadLimit(10 << 20) // 10MB

	// 创建 DaemonClient 并注册到 DaemonHub
	machineID := ""
	if machine != nil {
		machineID = machine.ID
	}
	client := ws.NewDaemonClient(conn, machineID)
	h.daemonHub.Register(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动写泵，用于向 daemon 发送消息
	go client.WritePump(ctx)

	// 启动服务端 ping 循环，检测死连接
	go h.serverPingLoop(ctx, client, machine)

	// 读取循环
	h.readLoop(ctx, client, machine)

	// 断开连接时注销
	h.daemonHub.Unregister(client)
	if machine != nil {
		h.agentSvc.MarkMachineOffline(machine.ID)
		// 仅当机器未重连时才推送 offline；否则新连接已接管，无需通知
		if !h.daemonHub.IsConnected(machine.ID) {
			ctxOff, cancelOff := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancelOff()
			agents, err := h.agentSvc.GetAgentsByMachine(ctxOff, machine.ID)
			if err == nil {
				for _, a := range agents {
					h.pushAgentStatus(a.ID, "offline")
				}
			}
		}
	}
}

// RegisterHTTP 处理 npx daemon 的一次性 HTTP 注册。
func (h *DaemonHandler) RegisterHTTP(c *gin.Context, machine *model.DaemonMachine) {
	var req struct {
		MachineID    string                    `json:"machine_id"`
		Agents       []service.DiscoveredAgent `json:"agents"`
		Capabilities []string                  `json:"capabilities"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40040, "message": "daemon 注册参数错误: " + err.Error(), "data": nil})
		return
	}

	registerCtx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if machine != nil {
		if len(req.Capabilities) > 0 {
			h.agentSvc.UpdateMachineCapabilities(machine.ID, req.Capabilities)
		}
		if err := h.agentSvc.RegisterMachineAgents(registerCtx, machine, req.MachineID, req.Agents); err != nil {
			h.logger.Error("register machine agents failed", "machine_id", req.MachineID, "machine", machine.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50040, "message": "注册电脑 Agent 失败", "data": nil})
			return
		}
		h.agentSvc.MarkMachineOnline(machine.ID)
		data := gin.H{"count": len(req.Agents)}
		if h.token != "" {
			data["daemon_token"] = h.token
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": data})
		return
	}

	if err := h.agentSvc.RegisterSystemAgents(registerCtx, req.MachineID, req.Agents); err != nil {
		h.logger.Error("register system agents failed", "machine_id", req.MachineID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50041, "message": "注册系统 Agent 失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"count": len(req.Agents)}})
}

// ClaimTask 让已连接电脑领取一条待执行的真实 CLI 任务。
func (h *DaemonHandler) ClaimTask(c *gin.Context, machine *model.DaemonMachine) {
	taskCtx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	task, err := h.agentSvc.ClaimDaemonTask(taskCtx, machine)
	if err != nil {
		h.logger.Error("claim daemon task failed", "machine", machine.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50042, "message": "领取 daemon 任务失败", "data": nil})
		return
	}
	h.agentSvc.TouchMachine(machine.ID)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": task})
}

// CompleteTask 接收电脑 daemon 对真实 CLI 任务的执行结果。
func (h *DaemonHandler) CompleteTask(c *gin.Context, machine *model.DaemonMachine) {
	var req struct {
		Result string `json:"result"`
		Error  string `json:"error"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40041, "message": "任务结果参数错误: " + err.Error(), "data": nil})
		return
	}

	taskCtx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := h.agentSvc.CompleteDaemonTask(taskCtx, machine, c.Param("id"), req.Result, req.Error); err != nil {
		h.logger.Error("complete daemon task failed", "machine", machine.ID, "task", c.Param("id"), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50043, "message": "提交 daemon 任务结果失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": nil})
}

// Heartbeat 接收 daemon 任务心跳，保持机器在线状态。
func (h *DaemonHandler) Heartbeat(c *gin.Context, machine *model.DaemonMachine) {
	h.agentSvc.TouchMachine(machine.ID)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": nil})
}

// IssueAgentToken 用机器 api-key 换取该机器所属用户的 agent_management scoped JWT，
// 供本机 MCP server 调用平台 REST API。仅接受 per-machine key（可映射到用户），
// 不接受全局 daemon token（无用户归属）。
func (h *DaemonHandler) IssueAgentToken(c *gin.Context, machine *model.DaemonMachine) {
	tokenCtx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	jwtToken, expiresAt, err := h.agentSvc.GenerateAgentToken(tokenCtx, machine.UserID)
	if err != nil {
		h.logger.Error("issue agent token failed", "machine", machine.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50044, "message": "签发 agent token 失败", "data": nil})
		return
	}
	h.agentSvc.TouchMachine(machine.ID)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{
		"token":      jwtToken,
		"expires_at": expiresAt.Format(time.RFC3339),
	}})
}

func (h *DaemonHandler) authenticateMachine(ctx context.Context, token string) (*model.DaemonMachine, error) {
	if token == "" {
		return nil, service.ErrAgentInvalidInput
	}
	if h.token != "" && token == h.token {
		return nil, nil
	}

	authCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return h.agentSvc.GetDaemonMachineByAPIKey(authCtx, token)
}

// serverPingLoop sends a {"type":"ping"} to the daemon every 30 seconds.
// If the write fails (client closed), it logs and cancels the context to close the connection.
func (h *DaemonHandler) serverPingLoop(ctx context.Context, client *ws.DaemonClient, machine *model.DaemonMachine) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.Send(ws.WSMessage{Type: "ping"}); err != nil {
				machineLabel := "<unknown>"
				if machine != nil {
					machineLabel = machine.ID
				}
				h.logger.Debug("server ping failed, closing daemon connection", "machine_id", machineLabel, "error", err)
				return
			}
		}
	}
}

func (h *DaemonHandler) readLoop(ctx context.Context, client *ws.DaemonClient, machine *model.DaemonMachine) {
	conn := client.Conn
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			h.logger.Debug("daemon websocket read end", "error", err)
			return
		}

		var envelope struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			h.logger.Warn("invalid daemon message", "error", err)
			continue
		}

		switch envelope.Type {
		case "daemon.register":
			h.handleRegister(ctx, client, envelope.Data, machine)
		case "task.complete":
			h.handleTaskComplete(envelope.Data, machine)
		case "task.progress":
			h.handleTaskProgress(envelope.Data, machine)
		case "agent.started":
			h.handleAgentStarted(envelope.Data)
		case "agent.stopped":
			h.handleAgentStopped(envelope.Data)
		case "ping":
			if err := client.Send(ws.WSMessage{Type: "pong"}); err != nil {
				h.logger.Warn("daemon pong failed", "error", err)
				return
			}
		case "pong":
			// Daemon responded to server ping — touch machine to confirm alive
			if machine != nil {
				h.agentSvc.TouchMachine(machine.ID)
			}
		default:
			h.logger.Warn("unknown daemon message", "type", envelope.Type)
		}
	}
}

func (h *DaemonHandler) handleTaskComplete(data json.RawMessage, machine *model.DaemonMachine) {
	var req struct {
		TaskID    string              `json:"task_id"`
		Result    string              `json:"result"`
		Error     string              `json:"error"`
		Artifacts []ws.ArtifactResult `json:"artifacts"`
		Cards     []map[string]any    `json:"cards"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		h.logger.Warn("invalid task.complete data", "error", err)
		return
	}
	h.logger.Info("handleTaskComplete ENTER", "task_id", req.TaskID, "result_len", len(req.Result), "has_error", req.Error != "", "cards_count", len(req.Cards))
	// Resolve WS promise first (for orchestrator channel-based wait)
	h.daemonHub.ResolveTask(req.TaskID, &ws.TaskResult{
		TaskID:    req.TaskID,
		Result:    req.Result,
		Error:     req.Error,
		Artifacts: req.Artifacts,
		Cards:     req.Cards,
	})
	// Also persist to DB (for HTTP fallback and audit)
	if machine != nil {
		taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.agentSvc.CompleteDaemonTask(taskCtx, machine, req.TaskID, req.Result, req.Error); err != nil {
			h.logger.Warn("persist task result failed", "task_id", req.TaskID, "error", err)
		}
	}

	// Orch worker results are now handled by goroutines using dispatchAndWait.
}

// handleTaskProgress 处理 daemon 的流式增量上报。
//
// daemon 侧 StreamBuffer 50ms 批量 flush AgentEvent[]，这里把批量事件透传给前端：
//   - WS 事件类型：TypeMessageStreaming ("message.streaming")
//   - payload: { message_id, conversation_id, deltas: AgentEvent[] }
//
// 前端按 message_id 路由到对应 streamingBlocks[messageId]，appendDelta 累积渲染。
//
// 注意：不写 DB（D3 ADR）——流式期间 backend 不 persist 增量，task.complete 时
// FinalizeStreaming 一次性落库最终 content/blocks_json。
func (h *DaemonHandler) handleTaskProgress(data json.RawMessage, machine *model.DaemonMachine) {
	var req struct {
		TaskID        string          `json:"task_id"`
		MessageID     string          `json:"message_id"`
		ConversationID string         `json:"conversation_id"`
		AgentID       string          `json:"agent_id"`
		Events        json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		h.logger.Warn("invalid task.progress data", "error", err)
		return
	}
	if req.MessageID == "" || len(req.Events) == 0 {
		// 没有 message_id 说明 backend dispatch 时未预创建 streaming message
		// （例如旧路径 / orchestrator / 非 agent reply）。事件无法路由，丢弃。
		h.logger.Debug("task.progress missing message_id or events, dropping",
			"task_id", req.TaskID, "event_bytes", len(req.Events))
		return
	}
	if h.userHub == nil {
		return
	}
	h.userHub.Broadcast(ws.WSMessage{
		Type: ws.TypeMessageStreaming,
		Data: map[string]interface{}{
			"message_id":      req.MessageID,
			"conversation_id": req.ConversationID,
			"task_id":         req.TaskID,
			"agent_id":        req.AgentID,
			"deltas":          json.RawMessage(req.Events), // 透传 AgentEvent[]，前端按 kind 渲染
		},
	})
}

func (h *DaemonHandler) handleRegister(ctx context.Context, client *ws.DaemonClient, data json.RawMessage, machine *model.DaemonMachine) {
	var req struct {
		MachineID    string                    `json:"machine_id"`
		Agents       []service.DiscoveredAgent `json:"agents"`
		Capabilities []string                  `json:"capabilities"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		h.logger.Warn("invalid daemon register data", "error", err)
		return
	}

	// 全局 token 连接时 machine == nil，但 daemon 会自报 machineID。
	// 用它更新 DaemonHub 注册，使 SendToMachine / IsConnected 可用。
	if machine == nil && req.MachineID != "" && client.MachineID != req.MachineID {
		h.daemonHub.UpdateMachineID(client, req.MachineID)
	}

	registerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if machine != nil {
		// 更新机器能力清单（docker 等），供部署策略选 machine 用
		if len(req.Capabilities) > 0 {
			h.agentSvc.UpdateMachineCapabilities(machine.ID, req.Capabilities)
		}
		if err := h.agentSvc.RegisterMachineAgents(registerCtx, machine, req.MachineID, req.Agents); err != nil {
			h.logger.Error("register machine agents failed", "machine_id", req.MachineID, "machine", machine.ID, "error", err)
			return
		}
		h.agentSvc.MarkMachineOnline(machine.ID)
		h.logger.Info("daemon machine agents registered", "machine_id", req.MachineID, "machine", machine.ID, "count", len(req.Agents))
		h.DispatchTask(&model.DaemonTask{MachineID: machine.ID})
		return
	}

	if err := h.agentSvc.RegisterSystemAgents(registerCtx, req.MachineID, req.Agents); err != nil {
		h.logger.Error("register system agents failed", "machine_id", req.MachineID, "error", err)
		return
	}
	h.logger.Info("daemon agents registered", "machine_id", req.MachineID, "count", len(req.Agents))
}

func (h *DaemonHandler) handleAgentStarted(data json.RawMessage) {
	var req struct {
		AgentID string `json:"agent_id"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		h.logger.Warn("invalid agent.started data", "error", err)
		return
	}
	if req.AgentID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	status := "online"
	if req.Error != "" {
		status = "error"
		h.logger.Warn("agent started with error", "agent_id", req.AgentID, "error", req.Error)
	}
	if err := h.agentSvc.SetAgentStatus(ctx, req.AgentID, status); err != nil {
		h.logger.Warn("update agent status after started failed", "agent_id", req.AgentID, "error", err)
	}
	h.pushAgentStatus(req.AgentID, status)
}

func (h *DaemonHandler) handleAgentStopped(data json.RawMessage) {
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		h.logger.Warn("invalid agent.stopped data", "error", err)
		return
	}
	if req.AgentID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := h.agentSvc.SetAgentStatus(ctx, req.AgentID, "stopped"); err != nil {
		h.logger.Warn("update agent status after stopped failed", "agent_id", req.AgentID, "error", err)
	}
	h.pushAgentStatus(req.AgentID, "stopped")
}

// pushAgentStatus pushes a real-time agent status update to the agent owner via WS.
// For system agents (no owner), broadcasts to all connected users.
func (h *DaemonHandler) pushAgentStatus(agentID, status string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	agent, err := h.agentSvc.GetAgentByID(ctx, agentID)
	if err != nil || agent == nil {
		return
	}
	msg := ws.WSMessage{
		Type: "agent.status",
		Data: map[string]string{
			"agent_id":     agentID,
			"agent_status": status,
		},
	}
	if agent.UserID != nil {
		h.userHub.SendToUser(*agent.UserID, msg)
	} else {
		h.userHub.Broadcast(msg)
	}
}
