package main

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/agent-hub/backend/internal/repository"
	"github.com/agent-hub/backend/internal/router"
	"github.com/agent-hub/backend/internal/service"
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
	platformSkillSvc.SetCatalogStore(platformSkillCatalogBridge{svc: catalogSvc})
	agentPromptTemplateSvc.SetCatalogStore(agentPromptCatalogBridge{svc: catalogSvc})
	userTemplateSvc.SetCatalogStore(userTemplateCatalogBridge{svc: catalogSvc})
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
	msgSvc.SetDeploymentService(deploymentSvc)

	if redisMsgRepo != nil {
		msgSvc.SetDeliveryState(redisMsgRepo)
	} else {
		msgSvc.SetDeliveryState(nil)
	}
	agentSvc.SetDaemonHub(daemonHub)
	msgSvc.SetDaemonHub(daemonHub)

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
	catalogHandler := catalog.NewHandler(catalogSvc)
	daemonHandler := handler.NewDaemonHandler(agentSvc, orchSvc, cfg.Daemon.Token, logger, cfg.CORS.AllowedOrigins, daemonHub, hub)
	agentRepo.SetDaemonTaskDispatcher(daemonHandler.DispatchTask)
	taskHandler := handler.NewTaskHandler(taskSvc, convRepo)
	artifactHandler := handler.NewArtifactHandler(artifactSvc)
	artifactHandler.SetOrchestratorService(orchSvc)
	deploymentHandler := handler.NewDeploymentHandler(deploymentSvc)
	knowledgeHandler := handler.NewKnowledgeHandler(knowledgeSvc, repository.NewGroupRepo(db))

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
		CatalogHandler:             catalogHandler,
		DaemonHandler:              daemonHandler,
		TaskHandler:                taskHandler,
		ArtifactHandler:            artifactHandler,
		DeploymentHandler:          deploymentHandler,
		KnowledgeHandler:           knowledgeHandler,
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

// toolDefinitionCatalogBridge adapts *catalog.Service to the
// service.ToolDefinitionCatalogLister interface declared locally in the
// service package. Declaring the adapter here (in package main, which
// already imports both catalog and service) breaks the would-be import
// cycle catalog → middleware → service → catalog.
type toolDefinitionCatalogBridge struct {
	svc *catalog.Service
}

func (b toolDefinitionCatalogBridge) ListToolDefinitions(ctx context.Context) ([]service.ToolDefinitionCatalogItem, error) {
	items, err := b.svc.List(ctx, catalog.DomainToolDefinition, catalog.ListQuery{})
	if err != nil {
		return nil, err
	}
	out := make([]service.ToolDefinitionCatalogItem, 0, len(items))
	for _, it := range items {
		out = append(out, service.ToolDefinitionCatalogItem{
			Name:        it.Key,
			Label:       it.Label,
			Category:    it.Category,
			Description: it.Description,
			CreatedAt:   it.CreatedAt,
		})
	}
	return out, nil
}

// platformSkillCatalogBridge adapts *catalog.Service to the
// service.PlatformSkillCatalogStore interface declared locally in the
// service package. Same pattern as toolDefinitionCatalogBridge but with
// full CRUD — each method maps catalog errors onto the legacy
// service.ErrPlatformSkill{NotFound,Invalid,Duplicate} sentinels so the
// handler-level error mapping (handlePlatformSkillError) keeps working
// without noticing that catalog is now the source of truth.
type platformSkillCatalogBridge struct {
	svc *catalog.Service
}

func (b platformSkillCatalogBridge) ListPlatformSkills(ctx context.Context, userID string) ([]service.PlatformSkillCatalogItem, error) {
	items, err := b.svc.List(ctx, catalog.DomainPlatformSkill, catalog.ListQuery{UserID: userID})
	if err != nil {
		return nil, mapCatalogToPlatformSkillErr(err)
	}
	out := make([]service.PlatformSkillCatalogItem, 0, len(items))
	for _, it := range items {
		out = append(out, platformSkillItemFromCatalog(it, userID))
	}
	return out, nil
}

func (b platformSkillCatalogBridge) CreatePlatformSkill(ctx context.Context, userID, name, category, description, trigger, detail string) (*service.PlatformSkillCatalogItem, error) {
	item, err := b.svc.Create(ctx, catalog.CreateInput{
		Domain:      catalog.DomainPlatformSkill,
		UserID:      userID,
		Key:         name,
		Label:       name,
		Category:    category,
		Description: description,
		PayloadJSON: mustJSONPayload(trigger, detail),
	})
	if err != nil {
		return nil, mapCatalogToPlatformSkillErr(err)
	}
	if item == nil {
		return nil, service.ErrPlatformSkillNotFound
	}
	out := platformSkillItemFromCatalog(*item, userID)
	return &out, nil
}

func (b platformSkillCatalogBridge) UpdatePlatformSkill(ctx context.Context, id, userID, name, category, description, trigger, detail string) (*service.PlatformSkillCatalogItem, error) {
	key := name
	label := name
	payload := mustJSONPayload(trigger, detail)
	item, err := b.svc.Update(ctx, id, catalog.UpdateInput{
		Domain:      catalog.DomainPlatformSkill,
		UserID:      userID,
		Key:         &key,
		Label:       &label,
		Category:    &category,
		Description: &description,
		PayloadJSON: &payload,
	})
	if err != nil {
		return nil, mapCatalogToPlatformSkillErr(err)
	}
	if item == nil {
		return nil, service.ErrPlatformSkillNotFound
	}
	out := platformSkillItemFromCatalog(*item, userID)
	return &out, nil
}

func (b platformSkillCatalogBridge) DeletePlatformSkill(ctx context.Context, id, userID string) error {
	if err := b.svc.Delete(ctx, catalog.DomainPlatformSkill, userID, id); err != nil {
		return mapCatalogToPlatformSkillErr(err)
	}
	return nil
}

// platformSkillItemFromCatalog maps a catalog.Item to the local DTO used
// by PlatformSkillService, decoding the {trigger, detail} payload. userID
// is taken from the caller (not Item.UserID) for parity with the legacy
// repo path, which always returns the caller's userID.
func platformSkillItemFromCatalog(it catalog.Item, userID string) service.PlatformSkillCatalogItem {
	trigger, detail := decodePlatformSkillPayload(it.PayloadJSON)
	uid := userID
	if it.UserID != nil && *it.UserID != "" {
		uid = *it.UserID
	}
	return service.PlatformSkillCatalogItem{
		ID:          it.ID,
		UserID:      uid,
		Name:        it.Key,
		Category:    it.Category,
		Description: it.Description,
		Trigger:     trigger,
		Detail:      detail,
		CreatedAt:   it.CreatedAt,
		UpdatedAt:   it.UpdatedAt,
	}
}

// mustJSONPayload serialises the {trigger, detail} pair as the catalog
// PayloadJSON. Errors are impossible for this shape, so we silently fall
// back to an empty object on marshal failure (defensive).
func mustJSONPayload(trigger, detail string) string {
	b, err := json.Marshal(map[string]string{"trigger": trigger, "detail": detail})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// decodePlatformSkillPayload is a main-package mirror of
// catalog.decodePlatformSkillPayload. Duplicated so the bridge doesn't
// reach into catalog's unexported helpers.
func decodePlatformSkillPayload(raw string) (trigger, detail string) {
	if strings.TrimSpace(raw) == "" {
		return "", ""
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", ""
	}
	return m["trigger"], m["detail"]
}

// mapCatalogToPlatformSkillErr translates catalog sentinel errors into
// the legacy PlatformSkill error sentinels. main package can import both,
// so we use errors.Is directly (no string matching).
func mapCatalogToPlatformSkillErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		return service.ErrPlatformSkillNotFound
	case errors.Is(err, catalog.ErrDuplicate):
		return service.ErrPlatformSkillDuplicate
	case errors.Is(err, catalog.ErrInvalid):
		return service.ErrPlatformSkillInvalid
	default:
		return fmt.Errorf("platform skill via catalog: %w", err)
	}
}

	// agentPromptCatalogBridge adapts *catalog.Service to the
	// service.AgentPromptTemplateCatalogStore interface.
	type agentPromptCatalogBridge struct {
		svc *catalog.Service
	}

	func (b agentPromptCatalogBridge) ListAgentPromptTemplates(ctx context.Context, userID string) ([]service.AgentPromptTemplateCatalogItem, error) {
		items, err := b.svc.List(ctx, catalog.DomainAgentPromptTemplate, catalog.ListQuery{UserID: userID})
		if err != nil {
			return nil, mapCatalogToAgentPromptErr(err)
		}
		out := make([]service.AgentPromptTemplateCatalogItem, 0, len(items))
		for _, it := range items {
			out = append(out, agentPromptItemFromCatalog(it, userID))
		}
		return out, nil
	}

	func (b agentPromptCatalogBridge) CreateAgentPromptTemplate(ctx context.Context, userID, name, category, description, systemPrompt string) (*service.AgentPromptTemplateCatalogItem, error) {
		item, err := b.svc.Create(ctx, catalog.CreateInput{
			Domain:      catalog.DomainAgentPromptTemplate,
			UserID:      userID,
			Key:         name,
			Label:       name,
			Category:    category,
			Description: description,
			PayloadJSON: mustJSONAgentPrompt(systemPrompt),
		})
		if err != nil {
			return nil, mapCatalogToAgentPromptErr(err)
		}
		if item == nil {
			return nil, service.ErrAgentPromptTemplateNotFound
		}
		out := agentPromptItemFromCatalog(*item, userID)
		return &out, nil
	}

	func (b agentPromptCatalogBridge) UpdateAgentPromptTemplate(ctx context.Context, id, userID, name, category, description, systemPrompt string) (*service.AgentPromptTemplateCatalogItem, error) {
		key := name
		label := name
		payload := mustJSONAgentPrompt(systemPrompt)
		item, err := b.svc.Update(ctx, id, catalog.UpdateInput{
			Domain:      catalog.DomainAgentPromptTemplate,
			UserID:      userID,
			Key:         &key,
			Label:       &label,
			Category:    &category,
			Description: &description,
			PayloadJSON: &payload,
		})
		if err != nil {
			return nil, mapCatalogToAgentPromptErr(err)
		}
		if item == nil {
			return nil, service.ErrAgentPromptTemplateNotFound
		}
		out := agentPromptItemFromCatalog(*item, userID)
		return &out, nil
	}

	func (b agentPromptCatalogBridge) DeleteAgentPromptTemplate(ctx context.Context, id, userID string) error {
		if err := b.svc.Delete(ctx, catalog.DomainAgentPromptTemplate, userID, id); err != nil {
			return mapCatalogToAgentPromptErr(err)
		}
		return nil
	}

	func agentPromptItemFromCatalog(it catalog.Item, userID string) service.AgentPromptTemplateCatalogItem {
		systemPrompt := decodeAgentPromptPayload(it.PayloadJSON)
		uid := userID
		if it.UserID != nil && *it.UserID != "" {
			uid = *it.UserID
		}
		return service.AgentPromptTemplateCatalogItem{
			ID:           it.ID,
			UserID:       uid,
			Name:         it.Key,
			Category:     it.Category,
			Description:  it.Description,
			SystemPrompt: systemPrompt,
			CreatedAt:    it.CreatedAt,
			UpdatedAt:    it.UpdatedAt,
		}
	}

	func mustJSONAgentPrompt(systemPrompt string) string {
		b, err := json.Marshal(map[string]string{"system_prompt": systemPrompt})
		if err != nil {
			return "{}"
		}
		return string(b)
	}

	func decodeAgentPromptPayload(raw string) string {
		if strings.TrimSpace(raw) == "" {
			return ""
		}
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return ""
		}
		return m["system_prompt"]
	}

	func mapCatalogToAgentPromptErr(err error) error {
		if err == nil {
			return nil
		}
		switch {
		case errors.Is(err, catalog.ErrNotFound):
			return service.ErrAgentPromptTemplateNotFound
		case errors.Is(err, catalog.ErrDuplicate):
			return service.ErrAgentPromptTemplateDuplicate
		case errors.Is(err, catalog.ErrInvalid):
			return service.ErrAgentPromptTemplateInvalid
		default:
			return fmt.Errorf("agent prompt template via catalog: %w", err)
		}
	}

	// userTemplateCatalogBridge adapts *catalog.Service to the
	// service.UserTemplateCatalogStore interface.
	type userTemplateCatalogBridge struct {
		svc *catalog.Service
	}

	func (b userTemplateCatalogBridge) ListUserTemplates(ctx context.Context, userID, tplType string) ([]service.UserTemplateCatalogItem, error) {
		items, err := b.svc.List(ctx, catalog.DomainUserTemplate, catalog.ListQuery{UserID: userID, Subtype: tplType})
		if err != nil {
			return nil, mapCatalogToUserTemplateErr(err)
		}
		out := make([]service.UserTemplateCatalogItem, 0, len(items))
		for _, it := range items {
			out = append(out, userTemplateItemFromCatalog(it, userID))
		}
		return out, nil
	}

	func (b userTemplateCatalogBridge) CreateUserTemplate(ctx context.Context, userID, tplType, name, content string) (*service.UserTemplateCatalogItem, error) {
		item, err := b.svc.Create(ctx, catalog.CreateInput{
			Domain:      catalog.DomainUserTemplate,
			UserID:      userID,
			Subtype:     tplType,
			Key:         name,
			Label:       name,
			PayloadJSON: content,
		})
		if err != nil {
			return nil, mapCatalogToUserTemplateErr(err)
		}
		if item == nil {
			return nil, service.ErrUserTemplateNotFound
		}
		out := userTemplateItemFromCatalog(*item, userID)
		return &out, nil
	}

	func (b userTemplateCatalogBridge) UpdateUserTemplate(ctx context.Context, id, userID, name, content string) (*service.UserTemplateCatalogItem, error) {
		key := name
		label := name
		payload := content
		item, err := b.svc.Update(ctx, id, catalog.UpdateInput{
			Domain:      catalog.DomainUserTemplate,
			UserID:      userID,
			Key:         &key,
			Label:       &label,
			PayloadJSON: &payload,
		})
		if err != nil {
			return nil, mapCatalogToUserTemplateErr(err)
		}
		if item == nil {
			return nil, service.ErrUserTemplateNotFound
		}
		out := userTemplateItemFromCatalog(*item, userID)
		return &out, nil
	}

	func (b userTemplateCatalogBridge) DeleteUserTemplate(ctx context.Context, id, userID string) error {
		if err := b.svc.Delete(ctx, catalog.DomainUserTemplate, userID, id); err != nil {
			return mapCatalogToUserTemplateErr(err)
		}
		return nil
	}

	func userTemplateItemFromCatalog(it catalog.Item, userID string) service.UserTemplateCatalogItem {
		uid := userID
		if it.UserID != nil && *it.UserID != "" {
			uid = *it.UserID
		}
		return service.UserTemplateCatalogItem{
			ID:        it.ID,
			UserID:    uid,
			Type:      it.Subtype,
			Name:      it.Key,
			Content:   it.PayloadJSON,
			CreatedAt: it.CreatedAt,
			UpdatedAt: it.UpdatedAt,
		}
	}

	func mapCatalogToUserTemplateErr(err error) error {
		if err == nil {
			return nil
		}
		switch {
		case errors.Is(err, catalog.ErrNotFound):
			return service.ErrUserTemplateNotFound
		case errors.Is(err, catalog.ErrDuplicate):
			return service.ErrUserTemplateDuplicate
		case errors.Is(err, catalog.ErrInvalid):
			return service.ErrUserTemplateInvalid
		default:
			return fmt.Errorf("user template via catalog: %w", err)
		}
	}
