package handler

import (
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	middleware "github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/repository"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// KnowledgeHandler 知识库接口处理器
type KnowledgeHandler struct {
	svc       *service.KnowledgeService
	groupRepo *repository.GroupRepo
}

// NewKnowledgeHandler 创建知识库处理器
func NewKnowledgeHandler(svc *service.KnowledgeService, groupRepo *repository.GroupRepo) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc, groupRepo: groupRepo}
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

// ListFiles 获取知识库中的文件列表（供 Agent 工具和前端使用）
func (h *KnowledgeHandler) ListFiles(c *gin.Context) {
	userID := middleware.GetUserID(c)
	kbID := c.Param("id")
	if kbID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40073, "缺少知识库 ID")
		return
	}

	files, err := h.svc.ListFiles(c.Request.Context(), userID, kbID)
	if err != nil {
		if err == service.ErrKBNotFound || err == service.ErrKBNoPermission {
			status := http.StatusNotFound
			code := 40467
			if err == service.ErrKBNoPermission {
				status = http.StatusForbidden
				code = 40365
			}
			middleware.ErrorResponse(c, status, code, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50069, "获取文件列表失败")
		return
	}
	if files == nil {
		files = []model.KnowledgeFile{}
	}
	middleware.SuccessResponse(c, files)
}

func (h *KnowledgeHandler) SearchFiles(c *gin.Context) {
	userID := middleware.GetUserID(c)
	kbID := c.Param("id")
	if kbID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40074, "缺少知识库 ID")
		return
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40075, "缺少搜索关键词")
		return
	}
	limit := 20
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = parsed
		}
	}
	results, err := h.svc.SearchFiles(c.Request.Context(), userID, kbID, keyword, limit)
	if err != nil {
		if err == service.ErrKBNotFound || err == service.ErrKBNoPermission {
			status := http.StatusNotFound
			code := 40468
			if err == service.ErrKBNoPermission {
				status = http.StatusForbidden
				code = 40366
			}
			middleware.ErrorResponse(c, status, code, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50070, "搜索知识库文件失败")
		return
	}
	middleware.SuccessResponse(c, results)
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
		if err == service.ErrKBNotFound || err == service.ErrKBNoPermission || err == service.ErrKBFileNotFound {
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

// SmartRenameFile 调用在线智能体扫读文件并更新知识库文件名。
func (h *KnowledgeHandler) SmartRenameFile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	kbID := c.Param("id")
	fileID := c.Param("fileId")
	if kbID == "" || fileID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40077, "缺少知识库 ID 或文件 ID")
		return
	}

	file, err := h.svc.SmartRenameFile(c.Request.Context(), userID, kbID, fileID)
	if err != nil {
		if err == service.ErrKBNotFound || err == service.ErrKBFileNotFound {
			status := http.StatusNotFound
			code := 40472
			if err == service.ErrKBFileNotFound {
				code = 40473
			}
			middleware.ErrorResponse(c, status, code, err.Error())
			return
		}
		if err == service.ErrKBNoPermission {
			middleware.ErrorResponse(c, http.StatusForbidden, 40368, err.Error())
			return
		}
		if err == service.ErrKBRenameNoAgent {
			middleware.ErrorResponse(c, http.StatusConflict, 40960, err.Error())
			return
		}
		if err == service.ErrMsgAgentTimeout {
			middleware.ErrorResponse(c, http.StatusGatewayTimeout, 50460, "智能体重命名超时")
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50072, "智能重命名失败")
		return
	}
	middleware.SuccessResponse(c, file)
}

// GetFileContent 获取文件内容（用于预览）
func (h *KnowledgeHandler) GetFileContent(c *gin.Context) {
	userID := middleware.GetUserID(c)
	kbID := c.Param("id")
	fileID := c.Param("fileId")
	if kbID == "" || fileID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40069, "缺少知识库 ID 或文件 ID")
		return
	}

	f, err := h.svc.GetFile(c.Request.Context(), userID, kbID, fileID)
	if err != nil {
		if err == service.ErrKBNotFound || err == service.ErrKBFileNotFound {
			status := http.StatusNotFound
			code := 40464
			if err == service.ErrKBFileNotFound {
				code = 40465
			}
			middleware.ErrorResponse(c, status, code, err.Error())
			return
		}
		if err == service.ErrKBNoPermission {
			middleware.ErrorResponse(c, http.StatusForbidden, 40364, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50066, "获取文件失败")
		return
	}

	if f.FilePath == "" {
		middleware.ErrorResponse(c, http.StatusNotFound, 40466, "文件不存在")
		return
	}

	absPath, err := service.SafeJoinUploadPath(h.svc.GetUploadDir(), f.FilePath)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusNotFound, 40466, "文件不存在")
		return
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		middleware.ErrorResponse(c, http.StatusNotFound, 40466, "文件不存在")
		return
	}

	// 图片和PDF inline预览，其他触发下载
	contentDisp := "attachment"
	if isPreviewMIME(f.MimeType) {
		contentDisp = "inline"
	}
	c.Header("Content-Disposition", contentDispositionHeader(contentDisp, f.Filename))
	c.Header("Content-Type", f.MimeType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(absPath)
}

func contentDispositionHeader(disposition, filename string) string {
	name := strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n':
			return -1
		default:
			return r
		}
	}, strings.TrimSpace(filename))
	if name == "" {
		name = "download"
	}
	header := mime.FormatMediaType(disposition, map[string]string{"filename": name})
	if header == "" {
		return disposition
	}
	return header
}

func (h *KnowledgeHandler) GetFileText(c *gin.Context) {
	userID := middleware.GetUserID(c)
	kbID := c.Param("id")
	fileID := c.Param("fileId")
	if kbID == "" || fileID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40076, "缺少知识库 ID 或文件 ID")
		return
	}
	result, err := h.svc.GetFileText(c.Request.Context(), userID, kbID, fileID)
	if err != nil {
		if err == service.ErrKBNotFound || err == service.ErrKBFileNotFound {
			status := http.StatusNotFound
			code := 40469
			if err == service.ErrKBFileNotFound {
				code = 40471
			}
			middleware.ErrorResponse(c, status, code, err.Error())
			return
		}
		if err == service.ErrKBNoPermission {
			middleware.ErrorResponse(c, http.StatusForbidden, 40367, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50071, "读取知识库文件文本失败")
		return
	}
	middleware.SuccessResponse(c, result)
}

// isPreviewMIME 判断MIME类型是否支持浏览器内预览
func isPreviewMIME(mime string) bool {
	previewTypes := map[string]bool{
		"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true,
		"text/plain": true, "text/markdown": true, "text/csv": true, "text/html": true,
		"application/pdf": true, "application/json": true,
	}
	return previewTypes[mime]
}

// ListGroup 获取群组中当前用户可用的知识库列表（自己的全部 + 其他成员的公开 KB）。
func (h *KnowledgeHandler) ListGroup(c *gin.Context) {
	userID := middleware.GetUserID(c)
	groupID := c.Param("groupId")
	if groupID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40072, "缺少群组 ID")
		return
	}

	members, err := h.groupRepo.ListMembers(c.Request.Context(), groupID)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50067, "获取群组成员失败")
		return
	}

	memberIDs := make([]string, 0, len(members))
	for _, m := range members {
		if m.UserID != "" {
			memberIDs = append(memberIDs, m.UserID)
		}
	}

	kbs, err := h.svc.ListGroupKnowledgeBases(c.Request.Context(), userID, memberIDs)
	if err != nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50068, "获取群组知识库列表失败")
		return
	}
	if kbs == nil {
		kbs = []model.KnowledgeBase{}
	}
	middleware.SuccessResponse(c, kbs)
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
