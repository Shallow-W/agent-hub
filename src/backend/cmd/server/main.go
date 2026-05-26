package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
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

	// 连接数据库，自动建库+自动迁移
	db, err := initDatabase(cfg, logger)
	if err != nil {
		logger.Error("init database failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// 依赖注入组装
	userRepo := repository.NewUserRepo(db)
	convRepo := repository.NewConversationRepo(db)
	msgRepo := repository.NewMessageRepo(db)
	friendRepo := repository.NewFriendRepo(db)

	authSvc := service.NewAuthService(userRepo, service.AuthConfig{
		JWTSecret:      cfg.JWT.Secret,
		JWTExpiryHours: cfg.JWT.ExpiryHours,
	})
	convSvc := service.NewConversationService(convRepo)
	msgSvc := service.NewMessageService(msgRepo, convRepo)
	friendSvc := service.NewFriendService(friendRepo)
	groupSvc := service.NewGroupService(repository.NewGroupRepo(db))

	hub := ws.NewHub(logger)
	authHandler := handler.NewAuthHandler(authSvc)
	convHandler := handler.NewConversationHandler(convSvc)
	msgHandler := handler.NewMessageHandler(msgSvc)
	friendHandler := handler.NewFriendHandler(friendSvc)
	groupHandler := handler.NewGroupHandler(groupSvc)
	wsHandler := handler.NewWebSocketHandler(authSvc, hub, logger, cfg.CORS.AllowedOrigins)

	// 路由设置
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORS(cfg.CORS.AllowedOrigins))
	router.Use(middleware.RequestLogger(logger))
	router.Use(middleware.RateLimit(100, 200))

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

	// 好友路由（需要鉴权）
	friendGroup := router.Group("/api/friends")
	friendGroup.Use(authMiddleware)
	{
		friendGroup.POST("/request", friendHandler.SendRequest)
		friendGroup.POST("/:id/accept", friendHandler.AcceptRequest)
		friendGroup.POST("/:id/reject", friendHandler.RejectRequest)
		friendGroup.GET("", friendHandler.ListFriends)
		friendGroup.GET("/pending", friendHandler.ListPending)
		friendGroup.GET("/search", friendHandler.SearchUsers)
	}

	// 群聊路由（需要鉴权）
	groupRoutes := router.Group("/api/groups")
	groupRoutes.Use(authMiddleware)
	{
		groupRoutes.POST("", groupHandler.CreateGroup)
		groupRoutes.GET("/:id", groupHandler.GetGroupInfo)
		groupRoutes.POST("/:id/members", groupHandler.AddMember)
		groupRoutes.DELETE("/:id/members/:userId", groupHandler.RemoveMember)
		groupRoutes.GET("/:id/members", groupHandler.ListMembers)
		groupRoutes.POST("/:id/leave", groupHandler.LeaveGroup)
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

// initDatabase 连接数据库，若不存在则自动创建并运行迁移
func initDatabase(cfg *Config, logger *slog.Logger) (*sqlx.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode,
	)

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		// 数据库不存在时自动创建
		if strings.Contains(err.Error(), "does not exist") {
			logger.Info("database not found, creating...", "dbname", cfg.Database.DBName)
			if err := createDatabase(cfg); err != nil {
				return nil, fmt.Errorf("create database: %w", err)
			}
			logger.Info("database created", "dbname", cfg.Database.DBName)
			db, err = sqlx.Connect("pgx", dsn)
			if err != nil {
				return nil, fmt.Errorf("connect after create: %w", err)
			}
		} else {
			return nil, fmt.Errorf("connect database: %w", err)
		}
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// 自动运行迁移
	if err := runMigrations(db, logger); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	logger.Info("database connected and migrated")
	return db, nil
}

// createDatabase 连接到默认 postgres 库并创建目标数据库
func createDatabase(cfg *Config) error {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=postgres sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.SSLMode,
	)
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer db.Close()

	// 防止 SQL 注入：数据库名来自配置文件，不接受用户输入
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", cfg.Database.DBName))
	return err
}

// runMigrations 从 migrations/ 目录读取 SQL 文件并执行（幂等）
func runMigrations(db *sqlx.DB, logger *slog.Logger) error {
	// 创建迁移追踪表
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR PRIMARY KEY,
		applied_at TIMESTAMPTZ DEFAULT NOW()
	)`)

	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Strings(files)

	for _, path := range files {
		name := filepath.Base(path)

		// 检查是否已执行
		var exists bool
		err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", name)
		if err != nil || exists {
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		// 只执行 ---- DOWN 之前的部分（UP migration）
		sql := string(content)
		if idx := strings.Index(sql, "---- DOWN"); idx != -1 {
			sql = sql[:idx]
		}

		if _, err := db.Exec(sql); err != nil {
			return fmt.Errorf("exec migration %s: %w", name, err)
		}

		_, _ = db.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", name)
		logger.Info("migration applied", "file", name)
	}

	return nil
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
