package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/agent-hub/backend/internal/ghpages"
	"github.com/agent-hub/backend/internal/handler"
	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/repository"
	"github.com/agent-hub/backend/internal/service"
	"github.com/agent-hub/backend/internal/tunnel"
	pkgredis "github.com/agent-hub/backend/pkg/redis"
	"github.com/agent-hub/backend/pkg/ws"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
)

func main() {
	// 加载配置
	cfg, err := loadConfig("config/config.yaml")
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}
	logLevel := parseLogLevel(cfg.Log.Level)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

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
	artifactRepo := repository.NewArtifactRepo(db)
	deploymentRepo := repository.NewDeploymentRepo(db)
	msgRepo := repository.NewMessageRepo(db, attachmentRepo, artifactRepo)
	friendRepo := repository.NewFriendRepo(db)
	agentRepo := repository.NewAgentRepo(db)
	platformSkillRepo := repository.NewPlatformSkillRepo(db)
	agentPromptTemplateRepo := repository.NewAgentPromptTemplateRepo(db)
	taskRepo := repository.NewTaskRepo(db)
	orchTaskRepo := repository.NewOrchTaskRepo(db)
	userTemplateRepo := repository.NewUserTemplateRepo(db)
	toolDefRepo := repository.NewToolDefinitionRepo(db)

	authSvc := service.NewAuthService(userRepo, service.AuthConfig{
		JWTSecret:      cfg.JWT.Secret,
		JWTExpiryHours: cfg.JWT.ExpiryHours,
	})
	convSvc := service.NewConversationService(convRepo, friendRepo)
	msgSvc := service.NewMessageService(msgRepo, convRepo, agentRepo)
	userSvc := service.NewUserService(userRepo)
	friendSvc := service.NewFriendService(friendRepo)
	groupSvc := service.NewGroupService(repository.NewGroupRepo(db))
	orchCardRepo := repository.NewOrchTaskCardRepo(db)
	taskSvc := service.NewTaskService(taskRepo)
	taskSvc.SetOrchCardRepo(orchCardRepo)
	artifactSvc := service.NewArtifactService(artifactRepo, convRepo)
	// PUBLIC_BASE_URL：配置内网穿透/公网入口时，部署预览与下载链接拼成绝对公网地址（二维码可扫、可分享）。
	deploymentSvc := service.NewDeploymentService(deploymentRepo, artifactRepo, convRepo, "", os.Getenv("PUBLIC_BASE_URL"))
	// GitHub Pages 永久发布（环境变量优先于 config.yaml）。三项齐全才启用，否则 publisher 为 nil。
	if pub := ghpages.NewPublisher(
		firstNonEmpty(os.Getenv("GITHUB_TOKEN"), cfg.GitHub.Token),
		firstNonEmpty(os.Getenv("GITHUB_PAGES_OWNER"), cfg.GitHub.Owner),
		firstNonEmpty(os.Getenv("GITHUB_PAGES_REPO"), cfg.GitHub.PagesRepo),
	); pub != nil {
		deploymentSvc.SetGitHubPublisher(pub)
		logger.Info("GitHub Pages 永久发布已启用", "owner", firstNonEmpty(os.Getenv("GITHUB_PAGES_OWNER"), cfg.GitHub.Owner), "repo", firstNonEmpty(os.Getenv("GITHUB_PAGES_REPO"), cfg.GitHub.PagesRepo))
	}
	fileURLs := service.NewFileURLBuilder(cfg.Upload.PublicBaseURL)
	knowledgeSvc := service.NewKnowledgeService(repository.NewKnowledgeRepo(db), userRepo, cfg.Upload.Dir, cfg.Upload.PublicBaseURL)

	// 文件上传服务
	uploadSvc := service.NewUploadService(service.UploadConfig{
		Dir:           cfg.Upload.Dir,
		MaxImageMB:    cfg.Upload.MaxImageMB,
		MaxPDFMB:      cfg.Upload.MaxPDFMB,
		PublicBaseURL: cfg.Upload.PublicBaseURL,
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
	}
	machineTracker := service.NewMachineTracker(agentRepo, logger)
	tokenIssuer := service.NewTokenIssuer(cfg.JWT.Secret)
	agentSvc := service.NewAgentService(agentRepo, machineTracker)
	platformSkillSvc := service.NewPlatformSkillService(platformSkillRepo)
	agentPromptTemplateSvc := service.NewAgentPromptTemplateService(agentPromptTemplateRepo)
	userTemplateSvc := service.NewUserTemplateService(userTemplateRepo)
	toolDefSvc := service.NewToolDefinitionService(toolDefRepo)
	agentSvc.SetTokenIssuer(tokenIssuer)
	agentSvc.SetServerURL(fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port))
	orchSvc := service.NewOrchestratorService(convRepo, agentRepo, msgRepo)
	orchSvc.SetTokenIssuer(tokenIssuer)
	orchSvc.SetServerURL(fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port))
	orchSvc.SetKBResolver(knowledgeSvc)
	orchSvc.SetArtifactRepo(artifactRepo)
	orchSvc.SetOrchTaskRepo(orchTaskRepo)
	orchSvc.SetTaskSvc(taskSvc)
	// 服务端抽取消息附件文本时需要定位上传文件。
	orchSvc.SetUploadDir(cfg.Upload.Dir)
	msgSvc.SetOrchestratorService(orchSvc)
	msgSvc.SetDeploymentService(deploymentSvc)
	msgSvc.SetFileURLBuilder(fileURLs)

	hub := ws.NewHub(logger)
	msgSvc.SetNotifier(hub)
	daemonHub := ws.NewDaemonHub(logger)
	orchSvc.SetDaemonHub(daemonHub)
	orchSvc.SetNotifier(hub)
	if rdb != nil {
		redisMsgRepo := repository.NewRedisMsgRepo(rdb)
		msgSvc.SetDeliveryState(redisMsgRepo)
		orchSvc.SetDeliveryState(redisMsgRepo)
	} else {
		msgSvc.SetDeliveryState(nil)
		orchSvc.SetDeliveryState(nil)
	}
	agentSvc.SetDaemonHub(daemonHub)
	msgSvc.SetDaemonHub(daemonHub)
	authHandler := handler.NewAuthHandler(authSvc)
	convHandler := handler.NewConversationHandler(convSvc)
	msgHandler := handler.NewMessageHandler(msgSvc)
	friendHandler := handler.NewFriendHandler(friendSvc)
	groupHandler := handler.NewGroupHandler(groupSvc)
	userHandler := handler.NewUserHandler(friendSvc, userSvc)
	uploadHandler := handler.NewUploadHandler(uploadSvc)
	pptPreviewHandler := handler.NewPptPreviewHandler(cfg.Upload.Dir)
	wsHandler := handler.NewWebSocketHandler(authSvc, hub, groupSvc, msgSvc, logger, cfg.CORS.AllowedOrigins)
	agentHandler := handler.NewAgentHandler(agentSvc, hub)
	platformSkillHandler := handler.NewPlatformSkillHandler(platformSkillSvc)
	agentPromptTemplateHandler := handler.NewAgentPromptTemplateHandler(agentPromptTemplateSvc)
	userTemplateHandler := handler.NewUserTemplateHandler(userTemplateSvc)
	toolDefHandler := handler.NewToolDefinitionHandler(toolDefSvc)
	daemonHandler := handler.NewDaemonHandler(agentSvc, orchSvc, cfg.Daemon.Token, logger, cfg.CORS.AllowedOrigins, daemonHub, hub)
	agentRepo.SetDaemonTaskDispatcher(daemonHandler.DispatchTask)
	taskHandler := handler.NewTaskHandler(taskSvc, convRepo)
	artifactHandler := handler.NewArtifactHandler(artifactSvc)
	artifactHandler.SetOrchestratorService(orchSvc)
	deploymentHandler := handler.NewDeploymentHandler(deploymentSvc)
	knowledgeHandler := handler.NewKnowledgeHandler(knowledgeSvc, repository.NewGroupRepo(db))

	// 路由设置
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.SetTrustedProxies(nil)
	router.Use(gin.Recovery())
	router.Use(middleware.CORS(cfg.CORS.AllowedOrigins))
	router.Use(middleware.RequestLogger(logger))

	// 健康检查（无需鉴权、不受限流）
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": nil})
	})
	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		healthy := true
		details := make(map[string]string)

		if err := db.PingContext(ctx); err != nil {
			healthy = false
			details["database"] = "unreachable: " + err.Error()
		} else {
			details["database"] = "ok"
		}

		if rdb != nil {
			if err := rdb.Ping(ctx).Err(); err != nil {
				healthy = false
				details["redis"] = "unreachable: " + err.Error()
			} else {
				details["redis"] = "ok"
			}
		} else {
			details["redis"] = "not_configured"
		}

		status := http.StatusOK
		if !healthy {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"code": 0, "message": "ready", "data": details})
	})

	// WebSocket 路由（通过 query 参数鉴权，不受限流）
	router.GET("/ws", wsHandler.Handle)

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
			filePath := cleanUploadRoutePath(c.Param("filepath"))
			if filePath == "" {
				c.Status(http.StatusForbidden)
				return
			}
			absPath, err := filepath.Abs(filepath.Join(uploadDir, filePath))
			uploadDirAbs, absErr := filepath.Abs(uploadDir)
			if err != nil {
				c.Status(http.StatusForbidden)
				return
			}
			if absErr != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
			if !strings.HasPrefix(absPath, uploadDirAbs+string(os.PathSeparator)) && absPath != uploadDirAbs {
				c.Status(http.StatusForbidden)
				return
			}

			// 缩略图和图片用 inline 预览，其他文件用 attachment 下载
			isThumbnail := strings.HasPrefix(filepath.ToSlash(filePath), "thumbnails/")
			ext := strings.ToLower(filepath.Ext(absPath))
			isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp"
			if isThumbnail || isImage {
				c.Header("Content-Disposition", "inline")
			} else {
				// 非图片文件触发浏览器下载，使用原始文件名
				fileName := filepath.Base(absPath)
				c.Header("Content-Disposition", "attachment; filename=\""+fileName+"\"")
			}
			c.Header("X-Content-Type-Options", "nosniff")
			c.File(absPath)
		})

		// 文件上传
		apiGroup.POST("/upload", uploadHandler.Upload)
		apiGroup.GET("/ppt-preview/*filepath", pptPreviewHandler.Preview)
		apiGroup.GET("/file-preview/*filepath", pptPreviewHandler.FilePreview)

		// 知识库路由（需要鉴权）
		kbRoutes := apiGroup.Group("/knowledge-bases")
		{
			kbRoutes.GET("", knowledgeHandler.List)
			kbRoutes.POST("", knowledgeHandler.Create)
			kbRoutes.PUT("/:id", knowledgeHandler.Update)
			kbRoutes.DELETE("/:id", knowledgeHandler.Delete)
			kbRoutes.POST("/:id/files", knowledgeHandler.UploadFile)
			kbRoutes.GET("/:id/files", knowledgeHandler.ListFiles)
			kbRoutes.GET("/:id/search", knowledgeHandler.SearchFiles)
			kbRoutes.GET("/:id/files/:fileId/text", knowledgeHandler.GetFileText)
			kbRoutes.GET("/:id/files/:fileId/content", knowledgeHandler.GetFileContent)
			kbRoutes.DELETE("/:id/files/:fileId", knowledgeHandler.DeleteFile)
			kbRoutes.GET("/group/:groupId", knowledgeHandler.ListGroup)
			kbRoutes.GET("/resolve", knowledgeHandler.ResolveKnowledgeRef)
		}

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
			convRoutes.POST("/:id/unarchive", convHandler.UnarchiveConversation)
			convRoutes.POST("/:id/pin", convHandler.TogglePin)
			convRoutes.POST("/:id/messages", msgHandler.Send)
			convRoutes.GET("/:id/messages", msgHandler.History)
			convRoutes.GET("/:id/messages/search", msgHandler.Search)
			convRoutes.GET("/:id/pinned-context", msgHandler.PinnedContext)
			convRoutes.GET("/:id/blackboard", msgHandler.GetBlackboard)
			convRoutes.PUT("/:id/blackboard", msgHandler.UpdateBlackboard)
			convRoutes.PUT("/:id/read", msgHandler.MarkAsRead)
			convRoutes.GET("/:id/messages/unread", msgHandler.Unread)
			convRoutes.POST("/:id/messages/:messageId/pin", msgHandler.Pin)
			convRoutes.DELETE("/:id/messages/:messageId/pin", msgHandler.Unpin)
			convRoutes.DELETE("/:id/messages/:messageId", msgHandler.Recall)
			convRoutes.GET("/:id/messages/:messageId/replies", msgHandler.Replies)
		}
		apiGroup.GET("/conversations/:id/agents", convHandler.ListAgents)
		apiGroup.POST("/conversations/agent", convHandler.GetOrCreateAgentPrivate)
		apiGroup.POST("/conversations/:id/agents", convHandler.AddAgent)
		apiGroup.DELETE("/conversations/:id/agents/:agentID", convHandler.RemoveAgent)
		apiGroup.PUT("/conversations/:id/agents/:agentID/role", convHandler.SetAgentRole)
		apiGroup.GET("/agents", agentHandler.List)
		apiGroup.POST("/agents", agentHandler.Create)
		apiGroup.PUT("/agents/:id", agentHandler.Update)
		apiGroup.PUT("/agents/:id/avatar", agentHandler.UpdateAvatar)
		apiGroup.PUT("/agents/:id/tags", agentHandler.UpdateTags)
		apiGroup.PUT("/agents/:id/custom-skills", agentHandler.UpdateCustomSkills)
		apiGroup.DELETE("/agents/:id", agentHandler.Delete)
		apiGroup.POST("/agent-tokens", agentHandler.GenerateAgentToken)
		apiGroup.POST("/agents/:id/start", agentHandler.StartAgent)
		apiGroup.POST("/agents/:id/restart", agentHandler.RestartAgent)
		apiGroup.POST("/agents/:id/stop", agentHandler.StopAgent)
		apiGroup.POST("/agents/:id/skills/open-location", agentHandler.OpenSkillLocation)
		apiGroup.GET("/platform-skills", platformSkillHandler.List)
		apiGroup.POST("/platform-skills", platformSkillHandler.Create)
		apiGroup.POST("/platform-skills/import-defaults", platformSkillHandler.ImportDefaults)
		apiGroup.PUT("/platform-skills/:id", platformSkillHandler.Update)
		apiGroup.DELETE("/platform-skills/:id", platformSkillHandler.Delete)
		apiGroup.GET("/agent-prompt-templates", agentPromptTemplateHandler.List)
		apiGroup.POST("/agent-prompt-templates", agentPromptTemplateHandler.Create)
		apiGroup.POST("/agent-prompt-templates/import-defaults", agentPromptTemplateHandler.ImportDefaults)
		apiGroup.PUT("/agent-prompt-templates/:id", agentPromptTemplateHandler.Update)
		apiGroup.DELETE("/agent-prompt-templates/:id", agentPromptTemplateHandler.Delete)
		apiGroup.GET("/user-templates", userTemplateHandler.List)
		apiGroup.POST("/user-templates", userTemplateHandler.Create)
		apiGroup.PUT("/user-templates/:id", userTemplateHandler.Update)
		apiGroup.DELETE("/user-templates/:id", userTemplateHandler.Delete)
		apiGroup.GET("/daemon/machines", agentHandler.ListDaemonMachines)
		apiGroup.POST("/daemon/machines", agentHandler.CreateDaemonMachine)
		apiGroup.DELETE("/daemon/machines/:id", agentHandler.DeleteDaemonMachine)
		apiGroup.GET("/daemon/machines/:id/connect", agentHandler.GetMachineConnect)
		apiGroup.GET("/daemon/agent-candidates", agentHandler.ListAgentCandidates)
		apiGroup.POST("/daemon/agent-candidates/:id/add", agentHandler.AddCandidateAgent)
		taskRoutes := apiGroup.Group("/tasks")
		taskRoutes.Use(middleware.ValidateUUIDParam("id"))
		{
			taskRoutes.GET("", taskHandler.List)
			taskRoutes.POST("", taskHandler.Create)
			taskRoutes.PUT("/:id", taskHandler.Update)
			taskRoutes.POST("/:id/status", taskHandler.MoveStatus)
			taskRoutes.DELETE("/:id", taskHandler.Delete)
			taskRoutes.GET("/orch-cards", taskHandler.ListOrchCards)
		}

		// 产物版本路由（需要鉴权，鉴权在 service 层按 rootId→对话成员校验）
		artifactRoutes := apiGroup.Group("/artifacts")
		artifactRoutes.Use(middleware.ValidateUUIDParam("rootId"))
		{
			artifactRoutes.GET("/:rootId/versions", artifactHandler.ListVersions)
			artifactRoutes.POST("/:rootId/versions", artifactHandler.CreateVersion)
			artifactRoutes.POST("/:rootId/ai-edit", artifactHandler.AIEdit)
			artifactRoutes.POST("/:rootId/deploy", deploymentHandler.Deploy)
			artifactRoutes.POST("/:rootId/deploy-github", deploymentHandler.DeployGitHub)
		}

		// 部署状态查询（需要鉴权）
		apiGroup.GET("/deployments/capabilities", deploymentHandler.Capabilities)
		apiGroup.GET("/deployments/:id", middleware.ValidateUUIDParam("id"), deploymentHandler.Get)
	}

	// 部署产物的公开访问（凭 deployment id 作能力令牌，无需鉴权，便于二维码/分享/直链下载）
	router.GET("/api/sites/:id/*filepath", middleware.ValidateUUIDParam("id"), deploymentHandler.ServeSite)
	router.GET("/api/deployments/:id/download", middleware.ValidateUUIDParam("id"), deploymentHandler.Download)
	// 带文件名末段的下载（:name 仅用于让浏览器存成正确的 .zip 名，handler 忽略其值）
	router.GET("/api/deployments/:id/download/:name", middleware.ValidateUUIDParam("id"), deploymentHandler.Download)

	// 好友路由（需要鉴权）
	friendGroup := router.Group("/api/friends")
	friendGroup.Use(authMiddleware)
	{
		friendGroup.POST("/request", friendHandler.SendRequest)
		friendGroup.POST("/:id/accept", middleware.ValidateUUIDParam("id"), friendHandler.AcceptRequest)
		friendGroup.POST("/:id/reject", middleware.ValidateUUIDParam("id"), friendHandler.RejectRequest)
		friendGroup.GET("", friendHandler.ListFriends)
		friendGroup.GET("/pending", friendHandler.ListPending)
		friendGroup.GET("/search", friendHandler.SearchUsers)
		friendGroup.DELETE("/:id", middleware.ValidateUUIDParam("id"), friendHandler.DeleteFriend)
	}

	// 群聊路由（需要鉴权）
	groupRoutes := router.Group("/api/groups")
	groupRoutes.Use(authMiddleware, middleware.ValidateUUIDParam("id"))
	{
		groupRoutes.POST("", groupHandler.CreateGroup)
		groupRoutes.GET("/:id", groupHandler.GetGroupInfo)
		groupRoutes.PUT("/:id", groupHandler.UpdateGroupInfo)
		groupRoutes.POST("/:id/members", groupHandler.AddMember)
		groupRoutes.DELETE("/:id/members/:userId", groupHandler.RemoveMember)
		groupRoutes.GET("/:id/members", groupHandler.ListMembers)
		groupRoutes.PUT("/:id/members/:memberId/role", groupHandler.ChangeMemberRole)
		groupRoutes.POST("/:id/leave", groupHandler.LeaveGroup)
		groupRoutes.POST("/:id/dissolve", groupHandler.DissolveGroup)
	}

	// 用户路由（需要鉴权）
	userGroup := router.Group("/api/users")
	userGroup.Use(authMiddleware)
	{
		userGroup.GET("/search", userHandler.Search)
		userGroup.GET("/me", userHandler.GetProfile)
		userGroup.PUT("/me", userHandler.UpdateProfile)
	}
	router.GET("/daemon/ws", daemonHandler.Handle)
	router.GET("/daemon/connect", daemonHandler.Handle)
	router.POST("/daemon/register", daemonHandler.WithMachine(daemonHandler.RegisterHTTP))
	router.GET("/daemon/agent-token", daemonHandler.WithMachine(daemonHandler.IssueAgentToken))
	router.GET("/daemon/tasks", daemonHandler.WithMachine(daemonHandler.ClaimTask))
	router.POST("/daemon/tasks/:id/complete", daemonHandler.WithMachine(daemonHandler.CompleteTask))
	router.POST("/daemon/tasks/:id/heartbeat", daemonHandler.WithMachine(daemonHandler.Heartbeat))

	// MCP 路由组（daemon token 认证，不依赖用户 JWT）
	mcpGroup := router.Group("/mcp")
	mcpGroup.Use(middleware.MCPAuth(cfg.Daemon.Token))
	{
		mcpGroup.GET("/conversations", convHandler.List)
		mcpGroup.GET("/conversations/:id/agents", convHandler.ListAgents)
		mcpGroup.GET("/conversations/:id/messages", msgHandler.History)

		mcpTaskRoutes := mcpGroup.Group("/tasks")
		mcpTaskRoutes.Use(middleware.ValidateUUIDParam("id"))
		{
			mcpTaskRoutes.GET("", taskHandler.List)
			mcpTaskRoutes.POST("", taskHandler.Create)
			mcpTaskRoutes.PUT("/:id", taskHandler.Update)
			mcpTaskRoutes.POST("/:id/status", taskHandler.MoveStatus)
			mcpTaskRoutes.DELETE("/:id", taskHandler.Delete)
		}

		mcpGroup.GET("/agents", agentHandler.MCPList)
		mcpGroup.POST("/agents", agentHandler.Create)
		mcpGroup.GET("/agents/:id", agentHandler.MCPGetAgentDetail)
		mcpGroup.PUT("/agents/:id", agentHandler.Update)
		mcpGroup.DELETE("/agents/:id", agentHandler.Delete)
		mcpGroup.POST("/agents/:id/start", agentHandler.StartAgent)
		mcpGroup.POST("/agents/:id/stop", agentHandler.StopAgent)
		mcpGroup.GET("/daemon/machines", agentHandler.ListDaemonMachines)
		mcpGroup.GET("/daemon/agent-candidates", agentHandler.ListAgentCandidates)
		mcpGroup.POST("/daemon/agent-candidates/:id/add", agentHandler.AddCandidateAgent)
		mcpGroup.POST("/groups", groupHandler.CreateGroup)
		mcpGroup.GET("/groups/:id", groupHandler.GetGroupInfo)
		mcpGroup.GET("/groups/:id/members", groupHandler.ListMembers)

		// 知识库
		mcpGroup.GET("/knowledge-bases", knowledgeHandler.List)
		mcpGroup.GET("/knowledge-bases/:id/files", knowledgeHandler.ListFiles)
		mcpGroup.GET("/knowledge-bases/:id/search", knowledgeHandler.SearchFiles)
		mcpGroup.GET("/knowledge-bases/:id/files/:fileId/text", knowledgeHandler.GetFileText)

		// 平台 Skills（只读）
		mcpGroup.GET("/platform-skills", platformSkillHandler.List)
		mcpGroup.POST("/platform-skills/import-defaults", platformSkillHandler.ImportDefaults)
	}

	// 工具定义和内置模板（公开元数据，无需鉴权）
	router.GET("/api/tools/definitions", toolDefHandler.ListDefinitions)
	router.GET("/api/tools/builtin-templates", toolDefHandler.ListBuiltinTemplates)

	registerSPARoutes(router, frontendDistDir())

	// 启动 Hub 事件循环
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	go daemonHub.Run(ctx)
	go machineTracker.Run(ctx)

	// 零配置内网穿透：未显式配置 PUBLIC_BASE_URL 且未禁用（AUTO_TUNNEL=false）时，
	// 自动拉起 cloudflared 快速隧道并把分配到的公网域名回填到部署服务——这样无论在哪台
	// 电脑启动项目，"部署"产出的预览/下载链接都天然是公网地址，无需手动配置。
	if os.Getenv("PUBLIC_BASE_URL") == "" && !isFalsy(os.Getenv("AUTO_TUNNEL")) {
		cacheDir := filepath.Join("data", "bin")
		go tunnel.StartAuto(ctx, cfg.Server.Port, cacheDir, logger, deploymentSvc.SetPublicBaseURL)
	}

	// 启动 HTTP 服务器
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 180 * time.Second,
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

	cancel() // stops Hub, DaemonHub, and MachineTracker
	daemonHub.Shutdown(shutdownCtx)

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

func frontendDistDir() string {
	if dir := os.Getenv("AGENTHUB_FRONTEND_DIST"); dir != "" {
		return dir
	}
	for _, dir := range frontendDistCandidates() {
		if info, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !info.IsDir() {
			return dir
		}
	}
	return filepath.Clean("../../frontend/dist")
}

func cleanUploadRoutePath(value string) string {
	cleaned := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if cleaned == "" || strings.HasPrefix(cleaned, "//") {
		return ""
	}
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = strings.TrimPrefix(cleaned, "uploads/")
	if cleaned == "" || strings.Contains(cleaned, ":") || strings.HasPrefix(cleaned, "/") || hasDotDotSegment(cleaned) {
		return ""
	}
	cleaned = path.Clean("/" + cleaned)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." || cleaned == "" || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return filepath.FromSlash(cleaned)
}

func hasDotDotSegment(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func frontendDistCandidates() []string {
	candidates := []string{
		filepath.Clean("src/frontend/dist"),
		filepath.Clean("../../frontend/dist"),
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "src", "frontend", "dist"),
			filepath.Join(exeDir, "frontend-dist"),
		)
	}
	return candidates
}
