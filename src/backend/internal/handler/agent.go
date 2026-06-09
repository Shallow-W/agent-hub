package handler

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/service"
	"github.com/agent-hub/backend/pkg/ws"
	"github.com/gin-gonic/gin"
)

// AgentHandler Agent 管理接口处理器
type AgentHandler struct {
	svc     *service.AgentService
	userHub *ws.Hub
}

// NewAgentHandler 创建 Agent 处理器
func NewAgentHandler(svc *service.AgentService, userHub *ws.Hub) *AgentHandler {
	return &AgentHandler{svc: svc, userHub: userHub}
}

// AgentRequest 自建 Agent 请求体
type AgentRequest struct {
	Name                  string `json:"name" binding:"required,max=100"`
	CLITool               string `json:"cli_tool" binding:"required,max=50"`
	SystemPrompt          string `json:"system_prompt"`
	ToolsConfig           string `json:"tools_config"`
	Avatar                string `json:"avatar"`
	CapabilitiesJSON      string `json:"capabilities_json"`
	EnableManagementTools bool   `json:"enable_management_tools"`
}

// CreateDaemonMachineRequest 创建远端电脑连接请求体
type CreateDaemonMachineRequest struct {
	Name string `json:"name" binding:"required,max=100"`
}

// CreateDaemonMachineResponse 返回给前端的连接凭据。
type CreateDaemonMachineResponse struct {
	Machine          *model.DaemonMachine `json:"machine"`
	APIKey           string               `json:"api_key"`
	DaemonSourcePath string               `json:"daemon_source_path"`
	DaemonNPMPath    string               `json:"daemon_npm_path"`
}

// AddCandidateAgentRequest 添加候选 Agent 请求体
type AddCandidateAgentRequest struct {
	Name         string `json:"name" binding:"required,max=100"`
	CLITool      string `json:"cli_tool" binding:"required,max=50"`
	SystemPrompt string `json:"system_prompt"`
	ToolsConfig  string `json:"tools_config"`
	CustomSkills string `json:"custom_skills"`
}

// List 查询可用 Agent 列表
func (h *AgentHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	list, err := h.svc.ListAvailable(c.Request.Context(), userID)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50030, "查询 Agent 列表失败")
		return
	}
	middleware.SuccessResponse(c, list)
}

// MCPList 查询 Agent 列表（瘦身体，去掉 capabilities_json 等大字段）
func (h *AgentHandler) MCPList(c *gin.Context) {
	userID := middleware.GetUserID(c)
	list, err := h.svc.ListAvailable(c.Request.Context(), userID)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50030, "查询 Agent 列表失败")
		return
	}
	slim := make([]gin.H, len(list))
	for i, a := range list {
		slim[i] = gin.H{
			"id":            a.ID,
			"name":          a.Name,
			"type":          a.Type,
			"status":        a.Status,
			"machine_id":    a.MachineID,
			"machine_name":  a.MachineName,
			"version":       a.Version,
			"cli_tool":      a.CLITool,
			"system_prompt": a.SystemPrompt,
			"tools_config":  a.ToolsConfig,
			"tags":          a.Tags,
			"custom_skills": a.CustomSkills,
		}
	}
	middleware.SuccessResponse(c, slim)
}

// MCPGetAgentDetail MCP 端点：查询单个 Agent 完整详情
func (h *AgentHandler) MCPGetAgentDetail(c *gin.Context) {
	agentID := c.Param("id")
	if agentID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40035, "缺少 Agent ID")
		return
	}
	agent, err := h.svc.GetAgentByID(c.Request.Context(), agentID)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50037, "查询 Agent 详情失败")
		return
	}
	if agent == nil {
		middleware.ErrorResponse(c, http.StatusNotFound, 40430, "Agent 不存在")
		return
	}
	middleware.SuccessResponse(c, agent)
}

// ListDaemonMachines 查询当前用户创建的电脑连接位。
func (h *AgentHandler) ListDaemonMachines(c *gin.Context) {
	userID := middleware.GetUserID(c)
	list, err := h.svc.ListDaemonMachines(c.Request.Context(), userID)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50034, "查询电脑连接列表失败")
		return
	}
	middleware.SuccessResponse(c, list)
}

// ListAgentCandidates 查询当前用户电脑上扫描到的候选 Agent。
func (h *AgentHandler) ListAgentCandidates(c *gin.Context) {
	userID := middleware.GetUserID(c)
	list, err := h.svc.ListAgentCandidates(c.Request.Context(), userID)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50036, "查询候选 Agent 失败")
		return
	}
	middleware.SuccessResponse(c, list)
}

// AddCandidateAgent 将候选 Agent 添加到 Agent 列表。
func (h *AgentHandler) AddCandidateAgent(c *gin.Context) {
	var req AddCandidateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40037, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	agent, err := h.svc.AddCandidateAgent(c.Request.Context(), userID, c.Param("id"), req.Name, req.CLITool, req.SystemPrompt, req.ToolsConfig, req.CustomSkills)
	if err != nil {
		middleware.HandleServiceError(c, err, "添加候选 Agent 失败")
		return
	}
	middleware.CreatedResponse(c, agent)
}

// CreateDaemonMachine 创建一台等待 daemon 接入的远端电脑。
func (h *AgentHandler) CreateDaemonMachine(c *gin.Context) {
	var req CreateDaemonMachineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40035, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	machine, apiKey, err := h.svc.CreateDaemonMachine(c.Request.Context(), userID, req.Name)
	if err != nil {
		middleware.HandleServiceError(c, err, "创建电脑连接失败")
		return
	}
	middleware.CreatedResponse(c, CreateDaemonMachineResponse{
		Machine:          machine,
		APIKey:           apiKey,
		DaemonSourcePath: resolveDaemonSourcePath(),
		DaemonNPMPath:    resolveDaemonNPMPath(),
	})
}

// DeleteDaemonMachine 删除电脑连接。
func (h *AgentHandler) DeleteDaemonMachine(c *gin.Context) {
	userID := middleware.GetUserID(c)
	err := h.svc.DeleteDaemonMachine(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		middleware.HandleServiceError(c, err, "删除电脑连接失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

// Create 创建自建 Agent
func (h *AgentHandler) Create(c *gin.Context) {
	var req AgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40030, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	agent, err := h.svc.CreateCustom(
		c.Request.Context(),
		userID,
		req.Name,
		req.CLITool,
		req.SystemPrompt,
		req.ToolsConfig,
		req.Avatar,
		req.CapabilitiesJSON,
		req.EnableManagementTools,
	)
	if err != nil {
		middleware.HandleServiceError(c, err, "创建 Agent 失败")
		return
	}
	middleware.CreatedResponse(c, agent)
}

// Update 更新自建 Agent
func (h *AgentHandler) Update(c *gin.Context) {
	agentID := c.Param("id")
	var req AgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40032, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	agent, err := h.svc.UpdateCustom(
		c.Request.Context(),
		agentID,
		userID,
		req.Name,
		req.CLITool,
		req.SystemPrompt,
		req.ToolsConfig,
		req.Avatar,
		req.CapabilitiesJSON,
		req.EnableManagementTools,
	)
	if err != nil {
		middleware.HandleServiceError(c, err, "更新 Agent 失败")
		return
	}
	middleware.SuccessResponse(c, agent)
}

// UpdateAvatarRequest 换头像请求体（仅 avatar 字段，无 required 约束）
type UpdateAvatarRequest struct {
	Avatar string `json:"avatar"`
}

// UpdateAvatar 仅更新 Agent 头像
func (h *AgentHandler) UpdateAvatar(c *gin.Context) {
	agentID := c.Param("id")
	var req UpdateAvatarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40044, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	agent, err := h.svc.UpdateAvatar(c.Request.Context(), agentID, userID, req.Avatar)
	if err != nil {
		middleware.HandleServiceError(c, err, "更新头像失败")
		return
	}
	middleware.SuccessResponse(c, agent)
}

// Delete 删除自建 Agent
func (h *AgentHandler) Delete(c *gin.Context) {
	agentID := c.Param("id")
	userID := middleware.GetUserID(c)

	err := h.svc.DeleteOwned(c.Request.Context(), agentID, userID)
	if err != nil {
		middleware.HandleServiceError(c, err, "删除 Agent 失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

// UpdateTagsRequest 更新标签请求体
type UpdateTagsRequest struct {
	Tags string `json:"tags"`
}

// UpdateTags 更新 Agent 标签（任意类型的 Agent 均可更新）
func (h *AgentHandler) UpdateTags(c *gin.Context) {
	agentID := c.Param("id")
	var req UpdateTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40033, "参数错误: "+err.Error())
		return
	}

	agent, err := h.svc.UpdateTags(c.Request.Context(), agentID, req.Tags)
	if err != nil {
		middleware.HandleServiceError(c, err, "更新标签失败")
		return
	}
	middleware.SuccessResponse(c, agent)
}

// UpdateCustomSkillsRequest 更新自定义技能请求体
type UpdateCustomSkillsRequest struct {
	CustomSkills string `json:"custom_skills"`
}

// UpdateCustomSkills 更新 Agent 自定义技能
func (h *AgentHandler) UpdateCustomSkills(c *gin.Context) {
	agentID := c.Param("id")
	var req UpdateCustomSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40034, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	agent, err := h.svc.UpdateCustomSkills(c.Request.Context(), agentID, userID, req.CustomSkills)
	if err != nil {
		middleware.HandleServiceError(c, err, "更新自定义技能失败")
		return
	}
	middleware.SuccessResponse(c, agent)
}

// AgentTokenResponse Agent Token 响应体
type AgentTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// GenerateAgentToken 生成 Agent 专用 JWT
func (h *AgentHandler) GenerateAgentToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	token, expiresAt, err := h.svc.GenerateAgentToken(c.Request.Context(), userID)
	if err != nil {
		middleware.HandleServiceError(c, err, "生成 Agent Token 失败")
		return
	}
	middleware.SuccessResponse(c, AgentTokenResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	})
}

// StartAgent 启动 Agent
func (h *AgentHandler) StartAgent(c *gin.Context) {
	agentID := c.Param("id")
	userID := middleware.GetUserID(c)
	err := h.svc.StartAgent(c.Request.Context(), agentID, userID)
	if err != nil {
		middleware.HandleServiceError(c, err, "启动 Agent 失败")
		return
	}
	h.pushAgentStatus(agentID, "online")
	middleware.SuccessResponse(c, map[string]string{"message": "agent started"})
}

// RestartAgent 重启 Agent
func (h *AgentHandler) RestartAgent(c *gin.Context) {
	agentID := c.Param("id")
	userID := middleware.GetUserID(c)
	err := h.svc.RestartAgent(c.Request.Context(), agentID, userID)
	if err != nil {
		middleware.HandleServiceError(c, err, "重启 Agent 失败")
		return
	}
	h.pushAgentStatus(agentID, "online")
	middleware.SuccessResponse(c, map[string]string{"message": "restart task created"})
}

// StopAgent 停止 Agent
func (h *AgentHandler) StopAgent(c *gin.Context) {
	agentID := c.Param("id")
	userID := middleware.GetUserID(c)
	err := h.svc.StopAgent(c.Request.Context(), agentID, userID)
	if err != nil {
		middleware.HandleServiceError(c, err, "停止 Agent 失败")
		return
	}
	h.pushAgentStatus(agentID, "offline")
	middleware.SuccessResponse(c, map[string]string{"message": "agent stopped"})
}

// GetMachineConnect 获取电脑连接命令
func (h *AgentHandler) GetMachineConnect(c *gin.Context) {
	machineID := c.Param("id")
	userID := middleware.GetUserID(c)
	command, machine, apiKey, err := h.svc.GetMachineConnectCommand(c.Request.Context(), machineID, userID)
	if err != nil {
		middleware.HandleServiceError(c, err, "获取连接命令失败")
		return
	}
	middleware.SuccessResponse(c, map[string]interface{}{
		"command":         command,
		"api_key":         apiKey,
		"daemon_npm_path": resolveDaemonNPMPath(),
		"machine":         machine,
	})
}

func resolveDaemonSourcePath() string {
	wd, err := os.Getwd()
	if err != nil {
		return "./src/daemon"
	}

	candidates := []string{
		filepath.Join(wd, "..", "daemon"),
		filepath.Join(wd, "src", "daemon"),
		filepath.Join(wd, "..", "..", "src", "daemon"),
	}
	for _, candidate := range candidates {
		goMod := filepath.Join(candidate, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return candidate
			}
			return abs
		}
	}

	return "./src/daemon"
}

func resolveDaemonNPMPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return "./src/daemon-npm"
	}

	candidates := []string{
		filepath.Join(wd, "..", "daemon-npm"),
		filepath.Join(wd, "src", "daemon-npm"),
		filepath.Join(wd, "..", "..", "src", "daemon-npm"),
	}
	for _, candidate := range candidates {
		manifest := filepath.Join(candidate, "package.json")
		if _, err := os.Stat(manifest); err == nil {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return candidate
			}
			return abs
		}
	}

	return "./src/daemon-npm"
}

// pushAgentStatus pushes a real-time agent status update to the agent owner via WS.
// For system agents (no owner), broadcasts to all connected users.
func (h *AgentHandler) pushAgentStatus(agentID, status string) {
	if h.userHub == nil {
		return
	}
	msg := ws.WSMessage{
		Type: "agent.status",
		Data: map[string]string{
			"agent_id":     agentID,
			"agent_status": status,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	agent, err := h.svc.GetAgentByID(ctx, agentID)
	if err != nil || agent == nil {
		return
	}
	if agent.UserID != nil {
		h.userHub.SendToUser(*agent.UserID, msg)
	} else {
		h.userHub.Broadcast(msg)
	}
}
