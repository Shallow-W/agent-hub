package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// 错误定义
var (
	ErrUserExists   = errors.New("用户名已存在")
	ErrInvalidInput = errors.New("参数无效")
	ErrAuthFailed   = errors.New("用户名或密码错误")
)

// AuthService 认证业务逻辑所需仓库接口
type UserRepo interface {
	CreateUser(ctx context.Context, username, passwordHash string) (*model.User, error)
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	GetUserByID(ctx context.Context, id string) (*model.User, error)
}

// AuthConfig 认证相关配置
type AuthConfig struct {
	JWTSecret   string
	JWTExpiryHours int
}

// AuthService 认证服务
type AuthService struct {
	repo   UserRepo
	config AuthConfig
}

// NewAuthService 创建认证服务
func NewAuthService(repo UserRepo, cfg AuthConfig) *AuthService {
	return &AuthService{repo: repo, config: cfg}
}

// Register 注册新用户，返回 JWT token 和用户信息
func (s *AuthService) Register(ctx context.Context, username, password string) (string, *model.User, error) {
	if username == "" || password == "" {
		return "", nil, fmt.Errorf("%w: 用户名和密码不能为空", ErrInvalidInput)
	}
	if len(username) < 3 || len(username) > 50 {
		return "", nil, fmt.Errorf("%w: 用户名长度需在 3-50 之间", ErrInvalidInput)
	}
	if len(password) < 6 {
		return "", nil, fmt.Errorf("%w: 密码长度不能少于 6 位", ErrInvalidInput)
	}

	// 检查用户名唯一性
	existing, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return "", nil, fmt.Errorf("check username: %w", err)
	}
	if existing != nil {
		return "", nil, ErrUserExists
	}

	// 密码哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.repo.CreateUser(ctx, username, string(hash))
	if err != nil {
		return "", nil, fmt.Errorf("create user: %w", err)
	}

	token, err := s.generateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}

	return token, user, nil
}

// Login 用户登录，验证密码并返回 JWT
func (s *AuthService) Login(ctx context.Context, username, password string) (string, *model.User, error) {
	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return "", nil, fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return "", nil, ErrAuthFailed
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, ErrAuthFailed
	}

	token, err := s.generateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}

	return token, user, nil
}

// generateToken 生成 JWT token
func (s *AuthService) generateToken(user *model.User) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"iat":      now.Unix(),
		"exp":      now.Add(time.Duration(s.config.JWTExpiryHours) * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWTSecret))
}

// ValidateToken 验证 JWT 并返回用户 ID
func (s *AuthService) ValidateToken(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.config.JWTSecret), nil
	})
	if err != nil {
		return "", fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", errors.New("invalid token")
	}
	userID, ok := claims["user_id"].(string)
	if !ok {
		return "", errors.New("missing user_id in token")
	}
	return userID, nil
}

// 确保 repository 实现满足接口
var _ UserRepo = (*repository.UserRepo)(nil)
