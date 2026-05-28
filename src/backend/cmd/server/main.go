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
	pkgredis "github.com/agent-hub/backend/pkg/redis"
	goredis "github.com/redis/go-redis/v9"
	"github.com/agent-hub/backend/pkg/ws"
	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
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
		Secret      string `koanf:"secret"`
		ExpiryHours int    `koanf:"expiry_hours"`
	} `koanf:"jwt"`
	CORS struct {
		AllowedOrigins []string `koanf:"allowed_origins"`
	} `koanf:"cors"`
	Redis struct {
		Host     string `koanf:"host"`
		Port     int    `koanf:"port"`
		Password string `koanf:"password"`
		DB       int    `koanf:"db"`
	} `koanf:"redis"`
	Upload struct {
		Dir        string `koanf:"dir"`
		MaxImageMB int    `koanf:"max_image_mb"`
		MaxPDFMB   int    `koanf:"max_pdf_mb"`
	} `koanf:"upload"`
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
	attachmentRepo := repository.NewAttachmentRepo(db)
	msgRepo := repository.NewMessageRepo(db, attachmentRepo)
	friendRepo := repository.NewFriendRepo(db)

	authSvc := service.NewAuthService(userRepo, service.AuthConfig{
		JWTSecret:      cfg.JWT.Secret,
		JWTExpiryHours: cfg.JWT.ExpiryHours,
	})
	convSvc := service.NewConversationService(convRepo, friendRepo)
	msgSvc := service.NewMessageService(msgRepo, convRepo)
	userSvc := service.NewUserService(userRepo)
	friendSvc := service.NewFriendService(friendRepo)
	groupSvc := service.NewGroupService(repository.NewGroupRepo(db))

	// 文件上传服务
	uploadSvc := service.NewUploadService(service.UploadConfig{
		Dir:        cfg.Upload.Dir,
		MaxImageMB: cfg.Upload.MaxImageMB,
		MaxPDFMB:   cfg.Upload.MaxPDFMB,
	})

	// Redis 初始化
	var rdb *goredis.Client
	rdb, err = pkgredis.NewClient(pkgredis.Config{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		logger.Warn("redis init failed, running without cache", "error", err)
	} else {
		logger.Info("redis connected")
		redisMsgRepo := repository.NewRedisMsgRepo(rdb)
		msgSvc.SetCacher(redisMsgRepo)
	}

	hub := ws.NewHub(logger)
	msgSvc.SetNotifier(hub)
	authHandler := handler.NewAuthHandler(authSvc)
	convHandler := handler.NewConversationHandler(convSvc)
	msgHandler := handler.NewMessageHandler(msgSvc)
	friendHandler := handler.NewFriendHandler(friendSvc)
	groupHandler := handler.NewGroupHandler(groupSvc)
	userHandler := handler.NewUserHandler(friendSvc, userSvc)
	uploadHandler := handler.NewUploadHandler(uploadSvc)
	wsHandler := handler.NewWebSocketHandler(authSvc, hub, groupSvc, msgSvc, logger, cfg.CORS.AllowedOrigins)

	// 路由设置
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.SetTrustedProxies(nil)
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
	apiGroup := router.Group("/api")
	apiGroup.Use(authMiddleware)
	{
		// 静态文件服务（上传的文件，需要鉴权）
		uploadDir := cfg.Upload.Dir
		if uploadDir == "" {
			uploadDir = "./uploads"
		}
		apiGroup.GET("/uploads/*filepath", func(c *gin.Context) {
			filePath := c.Param("filepath")
			cleaned := filepath.Clean(filePath)
			if strings.HasPrefix(cleaned, "..") {
				c.Status(http.StatusNotFound)
				return
			}
			c.Header("Content-Disposition", "inline")
			c.Header("X-Content-Type-Options", "nosniff")
			c.File(filepath.Join(uploadDir, cleaned))
		})

		// 文件上传
		apiGroup.POST("/upload", uploadHandler.Upload)

		// 会话路由（需要鉴权）
		convRoutes := apiGroup.Group("/conversations")
		convRoutes.Use(middleware.ValidateUUIDParam("id"), middleware.ValidateUUIDParam("messageId"))
		{
			convRoutes.POST("", convHandler.Create)
			convRoutes.POST("/private", convHandler.GetOrCreatePrivate)
			convRoutes.GET("", convHandler.List)
			convRoutes.GET("/archived", convHandler.ListArchived)
			convRoutes.PUT("/:id", convHandler.RenameConversation)
			convRoutes.DELETE("/:id", convHandler.Delete)
			convRoutes.POST("/:id/archive", convHandler.ArchiveConversation)
			convRoutes.POST("/:id/pin", convHandler.TogglePin)
			convRoutes.POST("/:id/messages", msgHandler.Send)
			convRoutes.GET("/:id/messages", msgHandler.History)
			convRoutes.GET("/:id/messages/search", msgHandler.Search)
			convRoutes.PUT("/:id/read", msgHandler.MarkAsRead)
			convRoutes.GET("/:id/messages/unread", msgHandler.Unread)
			convRoutes.DELETE("/:id/messages/:messageId", msgHandler.Recall)
		}
	}

	// 好友路由（需要鉴权）
	friendGroup := router.Group("/api/friends")
	friendGroup.Use(authMiddleware)
	{
		friendGroup.POST("/request", friendHandler.SendRequest)
		friendGroup.POST("/:id/accept", middleware.ValidateUUIDParam("id"), friendHandler.AcceptRequest)
		friendGroup.POST("/:id/reject", middleware.ValidateUUIDParam("id"), friendHandler.RejectRequest)
		friendGroup.GET("", friendHandler.ListFriends)
		friendGroup.GET("/pending", friendHandler.ListPending)
		friendGroup.GET("/search", middleware.RateLimit(10, 20), friendHandler.SearchUsers)
	}

	// 群聊路由（需要鉴权）
	groupRoutes := router.Group("/api/groups")
	groupRoutes.Use(authMiddleware, middleware.ValidateUUIDParam("id"))
	{
		groupRoutes.POST("", groupHandler.CreateGroup)
		groupRoutes.GET("/:id", groupHandler.GetGroupInfo)
		groupRoutes.POST("/:id/members", groupHandler.AddMember)
		groupRoutes.DELETE("/:id/members/:userId", groupHandler.RemoveMember)
		groupRoutes.GET("/:id/members", groupHandler.ListMembers)
		groupRoutes.POST("/:id/leave", groupHandler.LeaveGroup)
	}

	// 用户路由（需要鉴权）
	userGroup := router.Group("/api/users")
	userGroup.Use(authMiddleware)
	{
		userGroup.GET("/search", middleware.RateLimit(10, 20), userHandler.Search)
		userGroup.GET("/me", userHandler.GetProfile)
		userGroup.PUT("/me", userHandler.UpdateProfile)
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
	middleware.StopRateLimiters()

	if rdb != nil {
		if err := rdb.Close(); err != nil {
			logger.Warn("redis close failed", "error", err)
		}
	}

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

	// Quote identifier to prevent injection; name comes from config, not user input
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE \"%s\"", strings.ReplaceAll(cfg.Database.DBName, "\"", "")))
	return err
}

// runMigrations 从 migrations/ 目录读取 SQL 文件并执行（幂等）
func runMigrations(db *sqlx.DB, logger *slog.Logger) error {
	// 创建迁移追踪表
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR PRIMARY KEY,
		applied_at TIMESTAMPTZ DEFAULT NOW()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

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
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret is required")
	}
	if c.Server.Port <= 0 {
		return fmt.Errorf("server.port must be positive")
	}
	if c.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database.dbname is required")
	}
	return nil
}
