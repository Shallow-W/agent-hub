package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/agent-hub/backend/internal/catalog"
	"github.com/agent-hub/backend/internal/ghpages"
	"github.com/agent-hub/backend/internal/handler"
	wsinfra "github.com/agent-hub/backend/internal/infrastructure/ws"
	"github.com/agent-hub/backend/internal/middleware"
	"github.com/agent-hub/backend/internal/port"
	"github.com/agent-hub/backend/internal/repository"
	"github.com/agent-hub/backend/internal/router"
	"github.com/agent-hub/backend/internal/service"
	"github.com/agent-hub/backend/internal/service/tool_specs"
	"github.com/agent-hub/backend/internal/tunnel"
	pkgredis "github.com/agent-hub/backend/pkg/redis"
	"github.com/agent-hub/backend/pkg/ws"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
)

// detectLANIP 返回第一个非回环 IPv4 地址；检测失败返回空串。
func detectLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipNet, ok := a.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}
	return ""
}

// resolveExternalURL 确定 server 的外部可达 URL（供 daemon 连接、CORS 等）。
// 优先级：SERVER_EXTERNAL_URL 环境变量 > config server.external_url > 自动检测 LAN IP > 127.0.0.1。
func resolveExternalURL(cfg *Config) string {
	if u := firstNonEmpty(os.Getenv("SERVER_EXTERNAL_URL"), cfg.Server.ExternalURL); u != "" {
		return strings.TrimRight(u, "/")
	}
	if ip := detectLANIP(); ip != "" {
		return fmt.Sprintf("http://%s:%d", ip, cfg.Server.Port)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
}

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
	toolCategoryRepo := repository.NewToolCategoryRepo(db)

	// ToolRegistry: single source of truth for MCP tool definitions.
	// Auto-syncs to DB on register.
	toolRegistry := service.NewToolRegistry(toolDefRepo)
	registerAllToolSpecs(toolRegistry)

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
	deploymentSvc := service.NewDeploymentService(deploymentRepo, artifactRepo, convRepo, "", os.Getenv("PUBLIC_BASE_URL"))
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

	uploadSvc := service.NewUploadService(service.UploadConfig{
		Dir:           cfg.Upload.Dir,
		MaxImageMB:    cfg.Upload.MaxImageMB,
		MaxPDFMB:      cfg.Upload.MaxPDFMB,
		PublicBaseURL: cfg.Upload.PublicBaseURL,
	})

	// Redis 初始化（提前到 orchSvc 构造前，便于把 redisMsgRepo 直接注入 OrchestratorDeps）
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
	var redisMsgRepo *repository.RedisMsgRepo
	if rdb != nil {
		redisMsgRepo = repository.NewRedisMsgRepo(rdb)
	}
	machineTracker := service.NewMachineTracker(agentRepo, logger)
	tokenIssuer := service.NewTokenIssuer(cfg.JWT.Secret)
	agentSvc := service.NewAgentService(agentRepo, machineTracker)
	agentSvc.SetToolRegistry(toolRegistry)
	agentSvc.SetToolsetStore(toolDefRepo)
	platformSkillSvc := service.NewPlatformSkillService(platformSkillRepo)
	agentPromptTemplateSvc := service.NewAgentPromptTemplateService(agentPromptTemplateRepo)
	userTemplateSvc := service.NewUserTemplateService(userTemplateRepo)
	toolDefSvc := service.NewToolDefinitionService(toolDefRepo)

	// Catalog (B1): unified abstraction over the 4 directory vertical slices.
	// AdapterStore proxies to the existing repos (no DB changes); only
	// tool_definition currently routes through it — the other 3 domains keep
	// their legacy paths until B2/B3/B4.
	catalogRegistry := catalog.DefaultRegistry()
	catalogStore := catalog.NewAdapterStore(catalog.AdapterDeps{
		Plugins: map[catalog.Domain]catalog.DomainPlugin{
			catalog.DomainPlatformSkill:       catalog.NewPlatformSkillPlugin(platformSkillRepo),
			catalog.DomainToolDefinition:      catalog.NewToolDefinitionPlugin(toolDefRepo),
			catalog.DomainAgentPromptTemplate: catalog.NewAgentPromptTemplatePlugin(agentPromptTemplateRepo),
			catalog.DomainUserTemplate:        catalog.NewUserTemplatePlugin(userTemplateRepo),
		},
		Registry: catalogRegistry,
	})
	catalogSvc := catalog.NewService(catalogStore, catalogRegistry)
	toolDefSvc.SetCatalogLister(toolDefinitionCatalogBridge{svc: catalogSvc})
	platformSkillSvc.SetCatalogStore(newPlatformSkillBridge(catalogSvc))
	agentPromptTemplateSvc.SetCatalogStore(newAgentPromptBridge(catalogSvc))
	userTemplateSvc.SetCatalogStore(newUserTemplateBridge(catalogSvc))
	agentSvc.SetTokenIssuer(tokenIssuer)
	externalURL := resolveExternalURL(cfg)
	agentSvc.SetServerURL(externalURL)
	cfg.CORS.AllowedOrigins = append(cfg.CORS.AllowedOrigins, externalURL)

	hub := ws.NewHub(logger)
	msgSvc.SetNotifier(hub)
	eventBroadcaster := wsinfra.NewEventBroadcaster(hub, logger)
	roleSvc := service.NewRoleService(convRepo, eventBroadcaster)
	daemonHub := ws.NewDaemonHub(logger)

	// OrchestratorService: use the new deps struct instead of 10 setter calls.
	orchDeps := service.OrchestratorDeps{
		ConvRepo:     convRepo,
		AgentRepo:    agentRepo,
		MsgRepo:      msgRepo,
		OrchTaskRepo: orchTaskRepo,
		TokenIssuer:  tokenIssuer,
		ServerURL:    externalURL,
		UploadDir:    cfg.Upload.Dir,
		KBResolver:   knowledgeSvc,
		DaemonHub:    daemonHub,
		ArtifactRepo: artifactRepo,
		Notifier:     hub,
		TaskSvc:      taskSvc,
	}
	if redisMsgRepo != nil {
		orchDeps.Delivery = redisMsgRepo
	}
	orchSvc := service.NewOrchestratorServiceWithDeps(orchDeps)
	msgSvc.SetOrchestratorService(orchSvc)
	msgSvc.SetFileURLBuilder(fileURLs)

	if redisMsgRepo != nil {
		msgSvc.SetDeliveryState(redisMsgRepo)
	} else {
		msgSvc.SetDeliveryState(nil)
	}
	agentSvc.SetDaemonHub(daemonHub)
	knowledgeSvc.SetFilenameSuggester(agentSvc)
	msgSvc.SetDaemonHub(daemonHub)

	// TaskCardQueue: 收集 MCP subprocess 工具 emit 的卡片（如 deploy_project info 卡），
	// 按 task_id 索引。daemon subprocess 通过 POST /api/internal/task-cards 上报。
	// 在 createAgentReply 时 drain 合并到 message.cards_json。
	taskCardQueue := service.NewTaskCardQueue()
	msgSvc.SetTaskCardQueue(taskCardQueue)

	// PR4: StreamingBuffer 在 backend 内存累积 task_id → StreamingState。
	// daemon task.progress 时 handleTaskProgress 喂 events 给 buffer，
	// task.complete 时 createAgentReply GetState 拿到权威 blocks 落 blocks_json。
	// 单例：DaemonHandler 与 MessageService 共享同一实例。
	streamingBuffer := service.NewStreamingBuffer()
	msgSvc.SetStreamingBuffer(streamingBuffer)

	// Handlers
	authHandler := handler.NewAuthHandler(authSvc)
	convHandler := handler.NewConversationHandler(convSvc, roleSvc)
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
	toolDefHandler.SetToolRegistry(toolRegistry)
	toolCategoryHandler := handler.NewToolCategoryHandler(toolCategoryRepo)
	catalogHandler := catalog.NewHandler(catalogSvc)
	daemonHandler := handler.NewDaemonHandler(agentSvc, orchSvc, cfg.Daemon.Token, logger, cfg.CORS.AllowedOrigins, daemonHub, hub, streamingBuffer)
	// 不再注册 SetDaemonTaskDispatcher：CreateDaemonTask 的每个合法 caller
	// (createAgentReply / Dispatcher.dispatchCore / agent_browse / agent_skill_open)
	// 都会自己调 SendToMachine(task.dispatch) 并按需携带 message_id。
	// 若保留此回调，CreateDaemonTask 会同步触发 DispatchTask→SendToMachine，
	// 与 caller 自己的 SendToMachine 形成重复 dispatch：第一条不带 message_id，
	// 被 daemon 当作 shouldStream=false 处理，导致流式彻底失效（流式架构无 task.progress 事件）。
	taskHandler := handler.NewTaskHandler(taskSvc, convRepo)
	artifactHandler := handler.NewArtifactHandler(artifactSvc)
	artifactHandler.SetOrchestratorService(orchSvc)
	deploymentHandler := handler.NewDeploymentHandler(deploymentSvc)
	knowledgeHandler := handler.NewKnowledgeHandler(knowledgeSvc, repository.NewGroupRepo(db))
	internalHandler := handler.NewInternalHandler(taskCardQueue)

	// 路由设置
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(gin.Recovery())
	r.Use(middleware.CORS(cfg.CORS.AllowedOrigins))
	r.Use(middleware.RequestLogger(logger))

	// Health checks (depends on concrete DB/Redis types, stay in main)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": nil})
	})
	r.GET("/health/ready", func(c *gin.Context) {
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

	// WebSocket (authenticated via query params, no rate limit)
	r.GET("/ws", wsHandler.Handle)

	// All API, daemon, MCP routes
	router.Setup(r, router.Deps{
		AuthHandler:                authHandler,
		ConvHandler:                convHandler,
		MsgHandler:                 msgHandler,
		FriendHandler:              friendHandler,
		GroupHandler:               groupHandler,
		UserHandler:                userHandler,
		UploadHandler:              uploadHandler,
		PptPreviewHandler:          pptPreviewHandler,
		AgentHandler:               agentHandler,
		PlatformSkillHandler:       platformSkillHandler,
		AgentPromptTemplateHandler: agentPromptTemplateHandler,
		UserTemplateHandler:        userTemplateHandler,
		ToolDefHandler:             toolDefHandler,
		ToolCategoryHandler:        toolCategoryHandler,
		CatalogHandler:             catalogHandler,
		DaemonHandler:              daemonHandler,
		TaskHandler:                taskHandler,
		ArtifactHandler:            artifactHandler,
		DeploymentHandler:          deploymentHandler,
		KnowledgeHandler:           knowledgeHandler,
		InternalHandler:            internalHandler,
		JWTSecret:                  cfg.JWT.Secret,
		DaemonToken:                cfg.Daemon.Token,
		UploadDir:                  cfg.Upload.Dir,
	})

	// SPA fallback (depends on frontend build directory, stays in main)
	registerSPARoutes(r, frontendDistDir())

	// 启动 Hub 事件循环
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	go daemonHub.Run(ctx)
	go machineTracker.Run(ctx)
	// 启动 streaming watchdog：60s 超时阈值，10s 扫描间隔。
	// 兜底处理 daemon 崩溃 / WS 断开导致 streaming message 卡住的情况（R8 / D6）。
	// PR5：注入 hub + convRepo 让 watchdog 标记 stale 后广播 message.complete，
	// 前端立即感知 streaming 终态（status=error），不再永远 loading。
	streamingWatchdog := service.NewStreamingWatchdog(msgRepo, logger, 60*time.Second, 10*time.Second)
	streamingWatchdog.SetBroadcaster(hub, convRepo)
	go streamingWatchdog.Run(ctx)
	// 启动 TaskCardQueue 的 TTL 清理 goroutine（每小时扫一次过期 entry，防泄漏）。
	taskCardQueue.StartCleanup(ctx)

	// 零配置内网穿透
	if os.Getenv("PUBLIC_BASE_URL") == "" && !isFalsy(os.Getenv("AUTO_TUNNEL")) {
		cacheDir := filepath.Join("data", "bin")
		go tunnel.StartAuto(ctx, cfg.Server.Port, cacheDir, logger, deploymentSvc.SetPublicBaseURL)
	}

	// 启动 HTTP 服务器
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
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

// cleanUploadRoutePath sanitizes the upload route path parameter.
// Exported for use by spa_test.go.
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

// registerAllToolSpecs registers every MCP tool spec with the ToolRegistry
// at startup. This auto-syncs each tool's definition to the DB.
// To add a new tool: implement port.MCPToolSpec in tool_specs/ and add one line here.
func registerAllToolSpecs(registry *service.ToolRegistry) {
	ctx := context.Background()

	// Conversation tools
	mustRegister(ctx, registry, tool_specs.ListConversations())
	mustRegister(ctx, registry, tool_specs.ListConversationAgents())
	mustRegister(ctx, registry, tool_specs.GetMessages())
	mustRegister(ctx, registry, tool_specs.CreateGroup())

	// Task tools
	mustRegister(ctx, registry, tool_specs.ListTasks())
	mustRegister(ctx, registry, tool_specs.CreateTask())
	mustRegister(ctx, registry, tool_specs.UpdateTask())
	mustRegister(ctx, registry, tool_specs.MoveTaskStatus())
	mustRegister(ctx, registry, tool_specs.DeleteTask())

	// Agent tools
	mustRegister(ctx, registry, tool_specs.ListAgents())
	mustRegister(ctx, registry, tool_specs.ListAgentCandidates())
	mustRegister(ctx, registry, tool_specs.GetAgentDetail())
	mustRegister(ctx, registry, tool_specs.UpdateAgentPrompt())
	mustRegister(ctx, registry, tool_specs.StartAgent())
	mustRegister(ctx, registry, tool_specs.StopAgent())
	mustRegister(ctx, registry, tool_specs.CreateAgent())
	mustRegister(ctx, registry, tool_specs.UpdateAgent())
	mustRegister(ctx, registry, tool_specs.DeleteAgent())
	mustRegister(ctx, registry, tool_specs.ListToolsets())

	// Machine tools
	mustRegister(ctx, registry, tool_specs.ListMachines())

	// Group tools
	mustRegister(ctx, registry, tool_specs.GetGroupInfo())
	mustRegister(ctx, registry, tool_specs.ListGroupMembers())

	// Knowledge tools
	mustRegister(ctx, registry, tool_specs.ListKnowledgeBases())
	mustRegister(ctx, registry, tool_specs.ListKnowledgeFiles())
	mustRegister(ctx, registry, tool_specs.SearchKnowledge())
	mustRegister(ctx, registry, tool_specs.ReadKnowledgeFile())

	// Skill tools (new)
	mustRegister(ctx, registry, tool_specs.GetAgentSkill())
	mustRegister(ctx, registry, tool_specs.ListPlatformSkills())

	// Platform infrastructure tools（所有 Agent 默认可用，不可禁用）
	mustRegister(ctx, registry, tool_specs.RenderCard())
}

func mustRegister(ctx context.Context, registry *service.ToolRegistry, spec port.MCPToolSpec) {
	if err := registry.Register(ctx, spec); err != nil {
		slog.Error("register tool spec failed", "name", spec.Name(), "error", err)
		os.Exit(1)
	}
}
