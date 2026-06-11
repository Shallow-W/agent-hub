package catalog

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

// Handler is the unified REST entry point for catalog operations. It is
// mounted under /api/catalog/:domain and dispatches by HTTP method. The
// handler never switches on a specific Domain; all behavior is driven by
// the Service + Registry.
//
// Route map (see Register):
//
//	GET    /api/catalog/:domain              List
//	POST   /api/catalog/:domain              Create (user scope only)
//	GET    /api/catalog/:domain/:id          Get
//	PUT    /api/catalog/:domain/:id          Update
//	DELETE /api/catalog/:domain/:id          Delete
//	POST   /api/catalog/:domain/defaults     ImportDefaults
type Handler struct {
	svc *Service
	reg *Registry
}

// NewHandler wires the Handler with a Service. The Service's Registry is
// reused; pass nil-safe if the Service may be rebuilt.
func NewHandler(svc *Service) *Handler {
	h := &Handler{svc: svc}
	if svc != nil {
		h.reg = svc.Registry()
	}
	return h
}

// createRequest is the body for POST /:domain and PUT /:domain/:id.
type createRequest struct {
	Subtype     string `json:"subtype,omitempty"`
	Key         string `json:"key" binding:"required"`
	Label       string `json:"label,omitempty"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
	PayloadJSON string `json:"payload,omitempty"`
}

// updateRequest mirrors UpdateInput: pointer fields mean "leave unchanged".
// To support JSON null vs missing, callers may omit fields.
type updateRequest struct {
	Subtype     string  `json:"subtype,omitempty"`
	Key         *string `json:"key,omitempty"`
	Label       *string `json:"label,omitempty"`
	Category    *string `json:"category,omitempty"`
	Description *string `json:"description,omitempty"`
	PayloadJSON *string `json:"payload,omitempty"`
}

// Register mounts the catalog routes on the given gin RouterGroup. The
// group is expected to already carry whatever auth middleware the caller
// wants (JWT for user-scope, daemon token for system-scope read, etc.).
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/:domain", h.List)
	g.POST("/:domain", h.Create)
	g.GET("/:domain/:id", h.Get)
	g.PUT("/:domain/:id", h.Update)
	g.DELETE("/:domain/:id", h.Delete)
	g.POST("/:domain/defaults", h.ImportDefaults)
}

// DomainsHandler returns a JSON listing of all registered domains. Mount it
// once at GET /api/catalog (the parent of /:domain).
func (h *Handler) DomainsHandler(c *gin.Context) {
	if h.reg == nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50090, "catalog registry 未初始化")
		return
	}
	middleware.SuccessResponse(c, h.reg.Domains())
}

// List — GET /:domain
func (h *Handler) List(c *gin.Context) {
	domain := Domain(c.Param("domain"))
	q := ListQuery{
		UserID:   c.GetString("user_id"),
		Subtype:  c.Query("subtype"),
		Category: c.Query("category"),
	}
	items, err := h.svc.List(c.Request.Context(), domain, q)
	if err != nil {
		h.respondErr(c, err, "查询 catalog 列表失败")
		return
	}
	middleware.SuccessResponse(c, items)
}

// Create — POST /:domain (user scope only; system scope → 405)
func (h *Handler) Create(c *gin.Context) {
	domain := Domain(c.Param("domain"))
	spec, ok := h.spec(c, domain)
	if !ok {
		return
	}
	if spec.IsReadOnly() {
		middleware.ErrorResponse(c, http.StatusMethodNotAllowed, 40590, "该 catalog 域为只读")
		return
	}
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40090, "参数错误: "+err.Error())
		return
	}
	item, err := h.svc.Create(c.Request.Context(), CreateInput{
		Domain:      domain,
		UserID:      c.GetString("user_id"),
		Subtype:     req.Subtype,
		Key:         req.Key,
		Category:    req.Category,
		Label:       req.Label,
		Description: req.Description,
		PayloadJSON: req.PayloadJSON,
	})
	if err != nil {
		h.respondErr(c, err, "创建 catalog 条目失败")
		return
	}
	middleware.CreatedResponse(c, item)
}

// Get — GET /:domain/:id
func (h *Handler) Get(c *gin.Context) {
	item, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.respondErr(c, err, "查询 catalog 条目失败")
		return
	}
	middleware.SuccessResponse(c, item)
}

// Update — PUT /:domain/:id (user scope only)
func (h *Handler) Update(c *gin.Context) {
	domain := Domain(c.Param("domain"))
	spec, ok := h.spec(c, domain)
	if !ok {
		return
	}
	if spec.IsReadOnly() {
		middleware.ErrorResponse(c, http.StatusMethodNotAllowed, 40591, "该 catalog 域为只读")
		return
	}
	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40091, "参数错误: "+err.Error())
		return
	}
	item, err := h.svc.Update(c.Request.Context(), c.Param("id"), UpdateInput{
		Key:         req.Key,
		Label:       req.Label,
		Category:    req.Category,
		Description: req.Description,
		PayloadJSON: req.PayloadJSON,
	})
	if err != nil {
		h.respondErr(c, err, "更新 catalog 条目失败")
		return
	}
	middleware.SuccessResponse(c, item)
}

// Delete — DELETE /:domain/:id (user scope only)
func (h *Handler) Delete(c *gin.Context) {
	domain := Domain(c.Param("domain"))
	spec, ok := h.spec(c, domain)
	if !ok {
		return
	}
	if spec.IsReadOnly() {
		middleware.ErrorResponse(c, http.StatusMethodNotAllowed, 40592, "该 catalog 域为只读")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		h.respondErr(c, err, "删除 catalog 条目失败")
		return
	}
	middleware.SuccessResponse(c, nil)
}

// ImportDefaults — POST /:domain/defaults
func (h *Handler) ImportDefaults(c *gin.Context) {
	domain := Domain(c.Param("domain"))
	spec, ok := h.spec(c, domain)
	if !ok {
		return
	}
	if spec.IsReadOnly() {
		middleware.ErrorResponse(c, http.StatusMethodNotAllowed, 40593, "该 catalog 域为只读")
		return
	}
	items, err := h.svc.ImportDefaults(c.Request.Context(), domain, c.GetString("user_id"))
	if err != nil {
		h.respondErr(c, err, "导入默认值失败")
		return
	}
	middleware.SuccessResponse(c, items)
}

// spec fetches the DomainSpec and writes the appropriate HTTP error if the
// domain is unknown. Returns ok=false on error.
func (h *Handler) spec(c *gin.Context, d Domain) (DomainSpec, bool) {
	if h.reg == nil {
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50091, "catalog registry 未初始化")
		return DomainSpec{}, false
	}
	spec, ok := h.reg.Get(d)
	if !ok {
		middleware.ErrorResponse(c, http.StatusNotFound, 40490, "未知的 catalog 域: "+string(d))
		return DomainSpec{}, false
	}
	return spec, true
}

// respondErr maps a Service-layer error onto an HTTP response. Unified
// catalog sentinels get stable codes; unknown errors fall through to the
// generic middleware handler.
func (h *Handler) respondErr(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, ErrUnknownDomain):
		middleware.ErrorResponse(c, http.StatusNotFound, 40490, err.Error())
	case errors.Is(err, ErrNotFound):
		middleware.ErrorResponse(c, http.StatusNotFound, 40491, err.Error())
	case errors.Is(err, ErrInvalid):
		middleware.ErrorResponse(c, http.StatusBadRequest, 40092, err.Error())
	case errors.Is(err, ErrDuplicate):
		middleware.ErrorResponse(c, http.StatusConflict, 40990, err.Error())
	case errors.Is(err, ErrReadOnly):
		middleware.ErrorResponse(c, http.StatusMethodNotAllowed, 40594, err.Error())
	default:
		middleware.HandleServiceError(c, err, fallback)
	}
}
