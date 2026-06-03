package handler

import (
	"net/http"
	"strings"

	middleware "github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// KnowledgeHandler 知识库接口处理器
type KnowledgeHandler struct {
	svc *service.KnowledgeService
}

// NewKnowledgeHandler 创建知识库处理器
func NewKnowledgeHandler(svc *service.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc}
}

// CreateKnowledgeBaseRequest 创建知识库请求体
type CreateKnowledgeBaseRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// UpdateKnowledgeBaseRequest 更新知识库请求体
type UpdateKnowledgeBaseRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
}

// List 获取知识库列表
func (h *KnowledgeHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	kbs, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50060, "获取知识库列表失败")
		return
	}
	if kbs == nil {
		kbs = []model.KnowledgeBase{}
	}
	middleware.SuccessResponse(c, kbs)
}

// Create 创建知识库
func (h *KnowledgeHandler) Create(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req CreateKnowledgeBaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40060, "请求参数格式错误")
		return
	}

	kb, err := h.svc.Create(c.Request.Context(), userID, req.Name, req.Description)
	if err != nil {
		if err == service.ErrKBNameEmpty {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40061, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50061, "创建知识库失败")
		return
	}
	middleware.CreatedResponse(c, kb)
}

// Update 更新知识库
func (h *KnowledgeHandler) Update(c *gin.Context) {
	userID := middleware.GetUserID(c)
	kbID := c.Param("id")
	if kbID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40062, "缺少知识库 ID")
		return
	}

	var req UpdateKnowledgeBaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40063, "请求参数格式错误")
		return
	}

	if req.Visibility != "" {
		if err := h.svc.UpdateVisibility(c.Request.Context(), userID, kbID, req.Visibility); err != nil {
			if err == service.ErrKBNotFound || err == service.ErrKBNoPermission {
				status := http.StatusNotFound
				code := 40460
				if err == service.ErrKBNoPermission {
					status = http.StatusForbidden
					code = 40360
				}
				middleware.ErrorResponse(c, status, code, err.Error())
				return
			}
			middleware.ErrorResponse(c, http.StatusInternalServerError, 50062, "更新知识库失败")
			return
		}
	}
	middleware.SuccessResponse(c, gin.H{"id": kbID})
}

// Delete 删除知识库
func (h *KnowledgeHandler) Delete(c *gin.Context) {
	userID := middleware.GetUserID(c)
	kbID := c.Param("id")
	if kbID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40064, "缺少知识库 ID")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), userID, kbID); err != nil {
		if err == service.ErrKBNotFound || err == service.ErrKBNoPermission {
			status := http.StatusNotFound
			code := 40461
			if err == service.ErrKBNoPermission {
				status = http.StatusForbidden
				code = 40361
			}
			middleware.ErrorResponse(c, status, code, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50063, "删除知识库失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

// UploadFile 上传文件到知识库
func (h *KnowledgeHandler) UploadFile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	kbID := c.Param("id")
	if kbID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40065, "缺少知识库 ID")
		return
	}

	// 限制50MB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 50<<20)

	file, err := c.FormFile("file")
	if err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40066, "缺少上传文件")
		return
	}

	if err := h.svc.UploadFile(c.Request.Context(), userID, kbID, file); err != nil {
		if err == service.ErrKBNotFound || err == service.ErrKBNoPermission {
			status := http.StatusNotFound
			code := 40462
			if err == service.ErrKBNoPermission {
				status = http.StatusForbidden
				code = 40362
			}
			middleware.ErrorResponse(c, status, code, err.Error())
			return
		}
		if err == service.ErrKBFileEmpty {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40067, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50064, "上传文件失败")
		return
	}
	middleware.CreatedResponse(c, gin.H{"message": "上传成功"})
}

// DeleteFile 删除知识库文件
func (h *KnowledgeHandler) DeleteFile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	kbID := c.Param("id")
	fileID := c.Param("fileId")
	if kbID == "" || fileID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40068, "缺少知识库 ID 或文件 ID")
		return
	}

	if err := h.svc.DeleteFile(c.Request.Context(), userID, kbID, fileID); err != nil {
		if err == service.ErrKBNotFound || err == service.ErrKBNoPermission {
			status := http.StatusNotFound
			code := 40463
			if err == service.ErrKBNoPermission {
				status = http.StatusForbidden
				code = 40363
			}
			middleware.ErrorResponse(c, status, code, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50065, "删除文件失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

// ResolveKnowledgeRef 解析知识库引用（用于群聊中的 "用户名/知识库名" 语法）
func (h *KnowledgeHandler) ResolveKnowledgeRef(c *gin.Context) {
	userID := middleware.GetUserID(c)
	ref := c.Query("ref") // 格式: "用户名/知识库名"
	if ref == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40070, "缺少知识库引用")
		return
	}

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40071, "引用格式错误，正确格式为: 用户名/知识库名")
		return
	}

	kb, files, err := h.svc.ResolveKnowledgeRef(c.Request.Context(), userID, parts[0], parts[1])
	if err != nil {
		middleware.ErrorResponse(c, http.StatusNotFound, 40470, err.Error())
		return
	}

	type resolveResult struct {
		KnowledgeBase *model.KnowledgeBase  `json:"knowledge_base"`
		Files         []model.KnowledgeFile `json:"files"`
	}
	middleware.SuccessResponse(c, resolveResult{
		KnowledgeBase: kb,
		Files:         files,
	})
}

// maxUploadSize 知识库文件上传大小限制
const _ = 50 << 20
