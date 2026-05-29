package handler

import (
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// maxUploadSize 是 handler 层对请求体的硬上限（50MB），用于 MaxBytesReader 提前截断超大请求。
// 实际业务级大小检查（图片 20MB / PDF 50MB）由 UploadService.ProcessUpload 执行，
// 两层检查共同构成纵深防御：handler 层防止极端超大请求耗尽内存，service 层精确执行业务规则。
const maxUploadSize = 50 << 20 // 50MB，与后端 MaxPDFMB 一致

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
	// 在解析 multipart 前限制请求体大小
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)

	file, err := c.FormFile("file")
	if err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40050, "缺少上传文件")
		return
	}

	result, err := h.uploadSvc.ProcessUpload(c.Request.Context(), file)
	if err != nil {
		if err == service.ErrUploadEmpty || err == service.ErrUploadTypeInvalid {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40051, err.Error())
			return
		}
		if err == service.ErrUploadTooBig {
			middleware.ErrorResponse(c, http.StatusRequestEntityTooLarge, 40052, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50030, "文件上传失败")
		return
	}

	middleware.CreatedResponse(c, result)
}
