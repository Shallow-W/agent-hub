package handler

import (
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// UploadHandler 文件上传处理器
type UploadHandler struct {
	uploadSvc *service.UploadService
}

// NewUploadHandler 创建上传处理器
func NewUploadHandler(uploadSvc *service.UploadService) *UploadHandler {
	return &UploadHandler{uploadSvc: uploadSvc}
}

// Upload 处理文件上传
func (h *UploadHandler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40030, "缺少上传文件")
		return
	}

	result, err := h.uploadSvc.ProcessUpload(c.Request.Context(), file)
	if err != nil {
		if err == service.ErrUploadEmpty || err == service.ErrUploadTypeInvalid {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40031, err.Error())
			return
		}
		if err == service.ErrUploadTooBig {
			middleware.ErrorResponse(c, http.StatusRequestEntityTooLarge, 40032, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50030, "文件上传失败")
		return
	}

	middleware.CreatedResponse(c, result)
}
