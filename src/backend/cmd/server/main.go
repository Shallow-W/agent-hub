package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agent-hub/backend/internal/handler"
	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/repository"
	"github.com/agent-hub/backend/internal/service"
	"github.com/agent-hub/backend/pkg/ws"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
)

// Config 应用配置结构
type Config struct {
	Server struct {
		Port int `koanf:"port"`
	} `koanf:"server"`
	Database struct {
		Host     string `koanf:"host"`
		Port     int    `koanf:"port"`
		User     string `koanf:"user"`
		Password string `koanf:"password"`
		DBName   string `koanf:"dbname"`
		SSLMode  string `koanf:"sslmode"`
	} `koanf:"database"`
	JWT struct {
		Secret        string `koanf:"secret"`
		ExpiryHours   int    `koanf:"expiry_hours"`
	} `koanf:"jwt"`
	CORS struct {
		AllowedOrigins []string `koanf:"allowed_origins"`
	} `koanf:"cors"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// 加载配置
	cfg, err := loadConfig("config/config.yaml")
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}

	// 连接数据库
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode,
	)

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		logger.Error("connect database failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		logger.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	logger.Info("database connected")

	// 依赖注入组装
	userRepo := repository.NewUserRepo(db)
	convRepo := repository.NewConversationRepo(db)
	msgRepo := repository.NewMessageRepo(db)

	authSvc := service.NewAuthService(userRepo, service.AuthConfig{
		JWTSecret:      cfg.JWT.Secret,
		JWTExpiryHours: cfg.JWT.ExpiryHours,
	})
	convSvc := service.NewConversationService(convRepo)
	msgSvc := service.NewMessageService(msgRepo, convRepo)

	hub := ws.NewHub(logger)
	authHandler := handler.NewAuthHandler(authSvc)
	convHandler := handler.NewConversationHandler(convSvc)
	msgHandler := handler.NewMessageHandler(msgSvc)
	wsHandler := handler.NewWebSocketHandler(authSvc, hub, logger)

	// 路由设置
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORS(cfg.CORS.AllowedOrigins))

	// 健康检查（无需鉴权）
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": nil})
	})

	// 认证路由（无需鉴权）
	authGroup := router.Group("/api/auth")
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
	}

	// 需要鉴权的路由
	authMiddleware := middleware.Auth(middleware.JWTConfig{Secret: cfg.JWT.Secret})
	apiGroup := router.Group("/api").Use(authMiddleware)
	{
		apiGroup.POST("/conversations", convHandler.Create)
		apiGroup.GET("/conversations", convHandler.List)
		apiGroup.DELETE("/conversations/:id", convHandler.Delete)
		apiGroup.PUT("/conversations/:id/pin", convHandler.TogglePin)

		apiGroup.POST("/conversations/:id/messages", msgHandler.Send)
		apiGroup.GET("/conversations/:id/messages", msgHandler.History)
	}

	// WebSocket 路由（通过 query 参数鉴权）
	router.GET("/ws", wsHandler.Handle)

	// 启动 Hub 事件循环
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// 启动 HTTP 服务器
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("shutdown signal received", "signal", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	cancel() // 停止 Hub

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
	}

	logger.Info("server stopped")
}

// loadConfig 从 YAML 文件加载配置
func loadConfig(path string) (*Config, error) {
	k := koanf.New(".")
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("load config file: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
