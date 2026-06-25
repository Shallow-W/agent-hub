package handler

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// ArtifactHandler 处理产物版本接口。
type ArtifactHandler struct {
	svc     *service.ArtifactService
	orchSvc *service.OrchestratorService
}

// NewArtifactHandler 创建产物处理器。
func NewArtifactHandler(svc *service.ArtifactService) *ArtifactHandler {
	return &ArtifactHandler{svc: svc}
}

// SetOrchestratorService 注入编排服务，用于 AI 编辑产物（复用 daemon 派发）。
func (h *ArtifactHandler) SetOrchestratorService(orch *service.OrchestratorService) {
	h.orchSvc = orch
}

// CreateVersionRequest 创建产物新版本请求体。
type CreateVersionRequest struct {
	Content  string `json:"content"`
	Type     string `json:"type"`
	Language string `json:"language"`
	Filename string `json:"filename"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

// ListVersions 列出某血缘根的全部版本。
func (h *ArtifactHandler) ListVersions(c *gin.Context) {
	rootID := c.Param("rootId")
	if rootID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40700, "缺少产物 ID")
		return
	}

	userID := middleware.GetUserID(c)
	versions, err := h.svc.ListVersions(c.Request.Context(), rootID, userID)
	if err != nil {
		h.handleErr(c, err, "查询产物版本失败")
		return
	}
	middleware.SuccessResponse(c, versions)
}

// CreateVersion 为某血缘根创建新版本。
func (h *ArtifactHandler) CreateVersion(c *gin.Context) {
	rootID := c.Param("rootId")
	if rootID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40701, "缺少产物 ID")
		return
	}

	var req CreateVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40702, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	created, err := h.svc.CreateVersion(c.Request.Context(), rootID, userID, model.Artifact{
		Type:     req.Type,
		Language: req.Language,
		Filename: req.Filename,
		Title:    req.Title,
		URL:      req.URL,
		Content:  req.Content,
	})
	if err != nil {
		h.handleErr(c, err, "创建产物版本失败")
		return
	}
	middleware.CreatedResponse(c, created)
}

// AIEditRequest AI 编辑产物请求体。
type AIEditRequest struct {
	Instruction string `json:"instruction"`
	Selection   string `json:"selection"`
	// Version 指定编辑基准版本（>0 时用该版本内容作为 base；0 或缺省用最新版本）。
	Version int `json:"version"`
}

// AIEdit 用 AI 改写一个产物（code/webpage/document），结果存为新版本并返回。
func (h *ArtifactHandler) AIEdit(c *gin.Context) {
	rootID := c.Param("rootId")
	if rootID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40706, "缺少产物 ID")
		return
	}
	if h.orchSvc == nil {
		middleware.ErrorResponse(c, http.StatusServiceUnavailable, 50701, "AI 编辑能力未启用")
		return
	}

	var req AIEditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40707, "参数错误: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	created, err := h.orchSvc.AIEditArtifact(c.Request.Context(), rootID, userID, req.Instruction, req.Selection, req.Version)
	if err != nil {
		h.handleEditErr(c, err)
		return
	}
	middleware.CreatedResponse(c, created)
}

// handleErr 统一映射产物服务错误到 HTTP 响应。
func (h *ArtifactHandler) handleErr(c *gin.Context, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, service.ErrArtifactNotFound):
		middleware.ErrorResponse(c, http.StatusNotFound, 40703, err.Error())
	case errors.Is(err, service.ErrArtifactNoPerm):
		middleware.ErrorResponse(c, http.StatusForbidden, 40704, err.Error())
	case errors.Is(err, service.ErrArtifactInvalid):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40705, err.Error())
	default:
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50700, fallbackMsg)
	}
}

// handleEditErr 映射 AI 编辑错误到 HTTP 响应。
func (h *ArtifactHandler) handleEditErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrArtifactEditNotFound):
		middleware.ErrorResponse(c, http.StatusNotFound, 40708, err.Error())
	case errors.Is(err, service.ErrArtifactEditNoPerm):
		middleware.ErrorResponse(c, http.StatusForbidden, 40709, err.Error())
	case errors.Is(err, service.ErrArtifactEditInvalid):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40710, err.Error())
	case errors.Is(err, service.ErrArtifactEditUnsupported):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40711, err.Error())
	case errors.Is(err, service.ErrArtifactEditNoAgent):
		middleware.ErrorResponse(c, http.StatusConflict, 40901, err.Error())
	case errors.Is(err, service.ErrMsgAgentTimeout):
		middleware.ErrorResponse(c, http.StatusGatewayTimeout, 50402, err.Error())
	default:
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50702, "AI 编辑产物失败")
	}
}
