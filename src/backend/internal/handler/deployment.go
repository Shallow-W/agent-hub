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

// DeploymentHandler 处理产物部署发布接口。
type DeploymentHandler struct {
	svc *service.DeploymentService
}

// NewDeploymentHandler 创建部署处理器。
func NewDeploymentHandler(svc *service.DeploymentService) *DeploymentHandler {
	return &DeploymentHandler{svc: svc}
}

// Deploy 部署某血缘根的最新产物（需要鉴权，鉴权在 service 层按对话成员校验）。
func (h *DeploymentHandler) Deploy(c *gin.Context) {
	rootID := c.Param("rootId")
	if rootID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40800, "缺少产物 ID")
		return
	}
	userID := middleware.GetUserID(c)
	dep, err := h.svc.Deploy(c.Request.Context(), rootID, userID)
	if err != nil {
		h.handleErr(c, err)
		return
	}
	// url 保持相对路径，前端按当前来源（window.location.origin）拼绝对地址，适配二维码/局域网/生产同源。
	middleware.CreatedResponse(c, dep)
}

// DeployGitHub 把某血缘根的最新产物发布到 GitHub Pages（永久公网地址）。
func (h *DeploymentHandler) DeployGitHub(c *gin.Context) {
	rootID := c.Param("rootId")
	if rootID == "" {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40800, "缺少产物 ID")
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

// Get 查询部署状态（需要鉴权）。
func (h *DeploymentHandler) Get(c *gin.Context) {
	dep, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.handleErr(c, err)
		return
	}
	middleware.SuccessResponse(c, dep)
}

// ServeSite 公开静态服务已部署站点（凭 deployment id 访问，无需鉴权，便于二维码/分享）。
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
	// id 经路由 UUID 校验，rel 经 Clean + 前缀校验，双重防目录穿越。
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
	// 用 ServeContent 而非 c.File：避免 http.ServeFile 对 /index.html 的 301 重定向跳转。
	http.ServeContent(c.Writer, c.Request, fi.Name(), fi.ModTime(), f)
}

// Download 打包下载部署产物（公开，凭 deployment id 访问）。
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

// handleErr 统一映射部署服务错误到 HTTP 响应。
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
	default:
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50800, "部署失败")
	}
}
