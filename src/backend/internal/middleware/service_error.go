package middleware

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// serviceErrorMapping defines how a service-layer error maps to HTTP response.
type serviceErrorMapping struct {
	HTTPStatus int
	Code       int
	Message    string
}

// serviceErrorEntry is one entry in the error mapping table.
type serviceErrorEntry struct {
	Err     error
	Mapping serviceErrorMapping
}

// serviceErrorMappings is the central registry of service error -> HTTP response mappings.
// Handlers should use HandleServiceError instead of writing ad-hoc error chains.
var serviceErrorMappings = []serviceErrorEntry{
	// Task errors
	{service.ErrTaskNotFound, serviceErrorMapping{http.StatusNotFound, 40410, "任务不存在"}},
	{service.ErrTaskInvalid, serviceErrorMapping{http.StatusBadRequest, 40400, "任务参数错误"}},

	// Agent errors
	{service.ErrAgentNotFound, serviceErrorMapping{http.StatusNotFound, 40430, "Agent 不存在"}},
	{service.ErrAgentInvalidInput, serviceErrorMapping{http.StatusBadRequest, 40030, "Agent 参数无效"}},
	{service.ErrAgentOffline, serviceErrorMapping{http.StatusConflict, 40940, "Agent 不在线"}},
	{service.ErrMsgAgentNoPerm, serviceErrorMapping{http.StatusForbidden, 40330, "无权使用此 Agent"}},
	{service.ErrMsgAgentTimeout, serviceErrorMapping{http.StatusGatewayTimeout, 50430, "Agent 执行超时"}},

	// Conversation errors
	{service.ErrMsgConvNotFound, serviceErrorMapping{http.StatusNotFound, 40420, "对话不存在"}},
	{service.ErrMsgConvNoPerm, serviceErrorMapping{http.StatusForbidden, 40320, "无权操作此对话"}},
}

// HandleServiceError maps a service-layer error to an HTTP error response using the
// central mapping table. If no mapping matches, it falls back to 500 with the
// provided fallbackMsg. Returns true if an error was handled (caller should return).
func HandleServiceError(c *gin.Context, err error, fallbackMsg string) bool {
	for _, m := range serviceErrorMappings {
		if errors.Is(err, m.Err) {
			ErrorResponse(c, m.Mapping.HTTPStatus, m.Mapping.Code, m.Mapping.Message)
			return true
		}
	}
	ErrorResponse(c, http.StatusInternalServerError, 50000, fallbackMsg)
	return true
}
