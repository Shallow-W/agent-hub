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

type DeploymentCapabilities struct {
	GitHubEnabled bool `json:"github_enabled"`
}

func NewDeploymentHandler(svc *service.DeploymentService) *DeploymentHandler {
	return &DeploymentHandler{svc: svc}
}

// Capabilities returns publish features available in the current runtime.
func (h *DeploymentHandler) Capabilities(c *gin.Context) {
	middleware.SuccessResponse(c, DeploymentCapabilities{
		GitHubEnabled: h.svc.GitHubEnabled(),
	})
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
	default:
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50800, "deployment failed")
	}
}
