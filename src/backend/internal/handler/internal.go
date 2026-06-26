package handler

import (
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// InternalHandler 处理 daemon subprocess（MCP 工具子进程）回传给后端的内部回调，
// 例如 MCP 工具 emit 的卡片入队请求。
//
// 与 daemon.go 中的 daemon WS / HTTP handler 不同，这里处理的端点面向
// daemon 主进程 spawn 出来的 MCP 子进程——它们持有 daemon token 但
// 不持有 per-machine api-key，所以走 /api/internal/* + daemon-token Bearer。
type InternalHandler struct {
	taskCardQueue *service.TaskCardQueue
}

// NewInternalHandler 构造 InternalHandler。
func NewInternalHandler(taskCardQueue *service.TaskCardQueue) *InternalHandler {
	return &InternalHandler{taskCardQueue: taskCardQueue}
}

// pushTaskCardRequest POST /api/internal/task-cards 请求体。
type pushTaskCardRequest struct {
	TaskID string         `json:"task_id"`
	Card   map[string]any `json:"card"`
}

// PushTaskCard 处理 MCP subprocess 上报的卡片。
// 路由组已在 router.go 中套上 MCPAuth（daemon-token Bearer），到这里 token 已校验通过。
func (h *InternalHandler) PushTaskCard(c *gin.Context) {
	if h.taskCardQueue == nil {
		middleware.ErrorResponse(c, http.StatusServiceUnavailable, 50301, "task card queue 未启用")
		return
	}
	var req pushTaskCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40050, "task-cards 请求参数错误: "+err.Error())
		return
	}
	if req.TaskID == "" || req.Card == nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40051, "task_id 和 card 必填")
		return
	}
	h.taskCardQueue.Push(req.TaskID, req.Card)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": nil})
}
