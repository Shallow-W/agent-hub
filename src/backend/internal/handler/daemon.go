package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
	"nhooyr.io/websocket"
)

// DaemonHandler 处理本地守护进程连接
type DaemonHandler struct {
	agentSvc       *service.AgentService
	token          string
	logger         *slog.Logger
	allowedOrigins []string
	connMu         sync.RWMutex
	conns          map[string]*daemonConn
	dispatchMu     sync.Mutex
	dispatching    map[string]bool
	dispatchAgain  map[string]bool
}

// NewDaemonHandler 创建 daemon WebSocket 处理器
func NewDaemonHandler(agentSvc *service.AgentService, token string, logger *slog.Logger, allowedOrigins []string) *DaemonHandler {
	return &DaemonHandler{
		agentSvc:       agentSvc,
		token:          token,
		logger:         logger,
		allowedOrigins: allowedOrigins,
		conns:          make(map[string]*daemonConn),
		dispatching:    make(map[string]bool),
		dispatchAgain:  make(map[string]bool),
	}
}

// Handle 处理 daemon WebSocket 连接
func (h *DaemonHandler) Handle(c *gin.Context) {
	token := c.Query("token")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemonConn := &daemonConn{conn: conn}
	if machine != nil {
		h.registerDaemonConn(machine.ID, daemonConn)
		defer h.unregisterDaemonConn(machine.ID, daemonConn)
	}
	h.readLoop(ctx, daemonConn, machine)
}

// RegisterHTTP 处理 npx daemon 的一次性 HTTP 注册。
func (h *DaemonHandler) RegisterHTTP(c *gin.Context) {
	token := c.Query("token")
	machine, err := h.authenticateMachine(c.Request.Context(), token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40120, "message": "无效 daemon token", "data": nil})
		return
	}

	var req struct {
		MachineID string                    `json:"machine_id"`
		Agents    []service.DiscoveredAgent `json:"agents"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40040, "message": "daemon 注册参数错误: " + err.Error(), "data": nil})
		return
	}

	registerCtx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if machine != nil {
		if err := h.agentSvc.RegisterMachineAgents(registerCtx, machine, req.MachineID, req.Agents); err != nil {
			h.logger.Error("register machine agents failed", "machine_id", req.MachineID, "machine", machine.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50040, "message": "注册电脑 Agent 失败", "data": nil})
			return
		}
		h.agentSvc.MarkMachineOnline(machine.ID)
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"count": len(req.Agents)}})
		return
	}

	if err := h.agentSvc.RegisterSystemAgents(registerCtx, req.Agents); err != nil {
		h.logger.Error("register system agents failed", "machine_id", req.MachineID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50041, "message": "注册系统 Agent 失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"count": len(req.Agents)}})
}

// ClaimTask 让已连接电脑领取一条待执行的真实 CLI 任务。
func (h *DaemonHandler) ClaimTask(c *gin.Context) {
	token := c.Query("token")
	machine, err := h.authenticateMachine(c.Request.Context(), token)
	if err != nil || machine == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40121, "message": "无效 machine key", "data": nil})
		return
	}

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
func (h *DaemonHandler) CompleteTask(c *gin.Context) {
	token := c.Query("token")
	machine, err := h.authenticateMachine(c.Request.Context(), token)
	if err != nil || machine == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40122, "message": "无效 machine key", "data": nil})
		return
	}

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
func (h *DaemonHandler) Heartbeat(c *gin.Context) {
	token := c.Query("token")
	machine, err := h.authenticateMachine(c.Request.Context(), token)
	if err != nil || machine == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40123, "message": "无效 machine key", "data": nil})
		return
	}
	h.agentSvc.TouchMachine(machine.ID)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": nil})
}

// IssueAgentToken 用机器 api-key 换取该机器所属用户的 agent_management scoped JWT，
// 供本机 MCP server 调用平台 REST API。仅接受 per-machine key（可映射到用户），
// 不接受全局 daemon token（无用户归属）。
func (h *DaemonHandler) IssueAgentToken(c *gin.Context) {
	token := c.Query("token")
	machine, err := h.authenticateMachine(c.Request.Context(), token)
	if err != nil || machine == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40124, "message": "无效 machine key", "data": nil})
		return
	}

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

func (h *DaemonHandler) readLoop(ctx context.Context, daemonConn *daemonConn, machine *model.DaemonMachine) {
	for {
		_, data, err := daemonConn.conn.Read(ctx)
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
			h.handleRegister(ctx, envelope.Data, machine)
		case "task.done":
			h.handleTaskDone(ctx, envelope.Data, machine, false)
		case "task.error":
			h.handleTaskDone(ctx, envelope.Data, machine, true)
		case "task.heartbeat":
			h.handleTaskHeartbeat(envelope.Data, machine)
		case "ping":
			if err := daemonConn.write(ctx, []byte(`{"type":"pong"}`)); err != nil {
				h.logger.Warn("daemon pong failed", "error", err)
				return
			}
		default:
			h.logger.Warn("unknown daemon message", "type", envelope.Type)
		}
	}
}

func (h *DaemonHandler) handleRegister(ctx context.Context, data json.RawMessage, machine *model.DaemonMachine) {
	var req struct {
		MachineID string                    `json:"machine_id"`
		Agents    []service.DiscoveredAgent `json:"agents"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		h.logger.Warn("invalid daemon register data", "error", err)
		return
	}

	registerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if machine != nil {
		if err := h.agentSvc.RegisterMachineAgents(registerCtx, machine, req.MachineID, req.Agents); err != nil {
			h.logger.Error("register machine agents failed", "machine_id", req.MachineID, "machine", machine.ID, "error", err)
			return
		}
		h.agentSvc.MarkMachineOnline(machine.ID)
		h.logger.Info("daemon machine agents registered", "machine_id", req.MachineID, "machine", machine.ID, "count", len(req.Agents))
		h.startDaemonDispatch(machine.ID)
		return
	}

	if err := h.agentSvc.RegisterSystemAgents(registerCtx, req.Agents); err != nil {
		h.logger.Error("register system agents failed", "machine_id", req.MachineID, "error", err)
		return
	}
	h.logger.Info("daemon agents registered", "machine_id", req.MachineID, "count", len(req.Agents))
}
