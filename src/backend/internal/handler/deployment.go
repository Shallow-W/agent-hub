package handler

import (
	"archive/zip"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// DeploymentHandler handles artifact deployment and publishing.
type DeploymentHandler struct {
	svc *service.DeploymentService
}

// DeploymentCapabilities 描述当前运行时可用的发布能力。
// github_enabled 字段为前端历史契约，保留不变；strategies 字段为新增能力（mode → 是否启用），
// 供未来前端动态展示已注册策略（preview / github / cloudflare_pages ...）。
type DeploymentCapabilities struct {
	GitHubEnabled bool            `json:"github_enabled"`
	Strategies    map[string]bool `json:"strategies"`
}

func NewDeploymentHandler(svc *service.DeploymentService) *DeploymentHandler {
	return &DeploymentHandler{svc: svc}
}

// Capabilities returns publish features available in the current runtime.
// github_enabled 来自 "github" 策略的 Enabled()；strategies 遍历所有已注册策略。
func (h *DeploymentHandler) Capabilities(c *gin.Context) {
	strategies := make(map[string]bool)
	for _, p := range h.svc.Strategies() {
		strategies[p.Mode()] = p.Enabled()
	}
	middleware.SuccessResponse(c, DeploymentCapabilities{
		GitHubEnabled: strategies["github"], // 历史字段，保持向后兼容
		Strategies:    strategies,
	})
}

// DeployByConversationRequest 按 conversation_id 部署产物的请求体。
type DeployByConversationRequest struct {
	ConversationID string `json:"conversation_id" binding:"required"`
	ArtifactName   string `json:"artifact_name"`
	Mode           string `json:"mode"` // "preview"(default) | "github" | 未来其它已注册策略
}

// DeployByConversation 按 conversation_id + artifact_name 查找并部署产物。
// MCP 工具和聊天指令统一走此端点，不依赖 URL 中的 rootId。
// mode 派发全部走 service.DeployByMode：handler 不再有 switch mode，新增策略无需改这里。
func (h *DeploymentHandler) DeployByConversation(c *gin.Context) {
	var req DeployByConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40800, "invalid request: "+err.Error())
		return
	}
	userID := middleware.GetUserID(c)
	if req.Mode == "" {
		req.Mode = "preview"
	}
	dep, err := h.svc.DeployByMode(c.Request.Context(), req.Mode, req.ConversationID, userID, req.ArtifactName)
	if err != nil {
		h.handleErr(c, err)
		return
	}
	middleware.CreatedResponse(c, dep)
}

func (h *DeploymentHandler) Deploy(c *gin.Context) {
	rootID := c.Param("rootId")
	if rootID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40800, "missing artifact ID")
		return
	}
	userID := middleware.GetUserID(c)
	dep, err := h.svc.Deploy(c.Request.Context(), rootID, userID)
	if err != nil {
		h.handleErr(c, err)
		return
	}
	middleware.CreatedResponse(c, dep)
}

func (h *DeploymentHandler) DeployGitHub(c *gin.Context) {
	rootID := c.Param("rootId")
	if rootID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40800, "missing artifact ID")
		return
	}
	userID := middleware.GetUserID(c)
	dep, err := h.svc.PublishGitHub(c.Request.Context(), rootID, userID)
	if err != nil {
		h.handleErr(c, err)
		return
	}
	middleware.CreatedResponse(c, dep)
}

func (h *DeploymentHandler) Get(c *gin.Context) {
	dep, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.handleErr(c, err)
		return
	}
	middleware.SuccessResponse(c, dep)
}

func (h *DeploymentHandler) ServeSite(c *gin.Context) {
	id := c.Param("id")
	rel := c.Param("filepath")
	if rel == "" || rel == "/" {
		rel = "/index.html"
	}

	siteRoot, err := filepath.Abs(h.svc.SiteDir(id))
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	target, err := filepath.Abs(filepath.Join(siteRoot, filepath.Clean("/"+rel)))
	if err != nil || (!strings.HasPrefix(target, siteRoot+string(os.PathSeparator)) && target != siteRoot) {
		c.Status(http.StatusForbidden)
		return
	}
	f, openErr := os.Open(target)
	if openErr != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()
	fi, statErr := f.Stat()
	if statErr != nil || fi.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, fi.Name(), fi.ModTime(), f)
}

func (h *DeploymentHandler) Download(c *gin.Context) {
	dep, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	dir := h.svc.SiteDir(dep.ID)
	if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="deployment-`+dep.ID+`.zip"`)
	zw := zip.NewWriter(c.Writer)
	defer zw.Close()

	_ = filepath.Walk(dir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil || fi.IsDir() {
			return walkErr
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return nil
		}
		w, cerr := zw.Create(filepath.ToSlash(rel))
		if cerr != nil {
			return nil
		}
		f, oerr := os.Open(path)
		if oerr != nil {
			return nil
		}
		defer f.Close()
		_, _ = io.Copy(w, f)
		return nil
	})
}

func (h *DeploymentHandler) handleErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrDeployArtifactNotFound):
		middleware.ErrorResponse(c, http.StatusNotFound, 40801, err.Error())
	case errors.Is(err, service.ErrDeployNotFound):
		middleware.ErrorResponse(c, http.StatusNotFound, 40802, err.Error())
	case errors.Is(err, service.ErrDeployNoPerm):
		middleware.ErrorResponse(c, http.StatusForbidden, 40803, err.Error())
	case errors.Is(err, service.ErrDeployEmpty):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40804, err.Error())
	case errors.Is(err, service.ErrDeployNoArtifact):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40805, err.Error())
	case errors.Is(err, service.ErrGitHubNotConfigured):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40806, err.Error())
	case errors.Is(err, service.ErrDeployUnknownMode):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40807, err.Error())
	case errors.Is(err, service.ErrDeployModeNotConfigured):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40808, err.Error())
	default:
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50800, "deployment failed")
	}
}
