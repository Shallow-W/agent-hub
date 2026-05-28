package handler

import (
	"errors"
	"net/http"

	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// AuthHandler 认证接口处理器
type AuthHandler struct {
	svc *service.AuthService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// RegisterRequest 注册请求体
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=2,max=20"`
	Password string `json:"password" binding:"required,min=6,max=64"`
}

// LoginRequest 登录请求体
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// AuthResponse 认证成功响应体
type AuthResponse struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}

// Register 用户注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40001, "请求参数格式错误")
		return
	}

	token, user, err := h.svc.Register(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrUserExists) {
			middleware.ErrorResponse(c, http.StatusConflict, 40901, err.Error())
			return
		}
		if errors.Is(err, service.ErrInvalidInput) {
			middleware.ErrorResponse(c, http.StatusBadRequest, 40002, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50001, "注册失败")
		return
	}

	middleware.CreatedResponse(c, AuthResponse{Token: token, User: user})
}

// Login 用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ErrorResponse(c, http.StatusBadRequest, 40003, "请求参数格式错误")
		return
	}

	token, user, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrAuthFailed) {
			middleware.ErrorResponse(c, http.StatusUnauthorized, 40106, err.Error())
			return
		}
		middleware.ErrorResponse(c, http.StatusInternalServerError, 50002, "登录失败")
		return
	}

	middleware.SuccessResponse(c, AuthResponse{Token: token, User: user})
}
