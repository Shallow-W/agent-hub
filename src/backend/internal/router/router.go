package router

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/agent-hub/backend/internal/catalog"
	"github.com/agent-hub/backend/internal/handler"
	"github.com/agent-hub/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

// Deps aggregates all HTTP handler instances needed for route setup.
type Deps struct {
	AuthHandler                *handler.AuthHandler
	ConvHandler                *handler.ConversationHandler
	MsgHandler                 *handler.MessageHandler
	FriendHandler              *handler.FriendHandler
	GroupHandler               *handler.GroupHandler
	UserHandler                *handler.UserHandler
	UploadHandler              *handler.UploadHandler
	PptPreviewHandler          *handler.PptPreviewHandler
	AgentHandler               *handler.AgentHandler
	PlatformSkillHandler       *handler.PlatformSkillHandler
	AgentPromptTemplateHandler *handler.AgentPromptTemplateHandler
	UserTemplateHandler        *handler.UserTemplateHandler
	ToolDefHandler             *handler.ToolDefinitionHandler
	ToolCategoryHandler        *handler.ToolCategoryHandler
	CatalogHandler             *catalog.Handler
	DaemonHandler              *handler.DaemonHandler
	TaskHandler                *handler.TaskHandler
	ArtifactHandler            *handler.ArtifactHandler
	DeploymentHandler          *handler.DeploymentHandler
	KnowledgeHandler           *handler.KnowledgeHandler

	// Config values needed for route setup
	JWTSecret   string
	DaemonToken string
	UploadDir   string
}

// Setup registers all HTTP routes on the given gin Engine.
// SPA fallback and health checks are not registered here – main.go handles them directly.
func Setup(r *gin.Engine, deps Deps) {
	// Auth routes (no auth)
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/register", deps.AuthHandler.Register)
		authGroup.POST("/login", deps.AuthHandler.Login)
	}

	// Authenticated API routes
	authMiddleware := middleware.Auth(middleware.JWTConfig{Secret: deps.JWTSecret})
	apiGroup := r.Group("/api")
	apiGroup.Use(authMiddleware)
	{
		// Static file serving for uploads (authenticated)
		uploadDir := deps.UploadDir
		if uploadDir == "" {
			uploadDir = "./uploads"
		}
		apiGroup.GET("/uploads/*filepath", makeUploadHandler(uploadDir))

		// File upload
		apiGroup.POST("/upload", deps.UploadHandler.Upload)
		apiGroup.GET("/ppt-preview/*filepath", deps.PptPreviewHandler.Preview)
		apiGroup.GET("/file-preview/*filepath", deps.PptPreviewHandler.FilePreview)

		// Knowledge base routes
		kbRoutes := apiGroup.Group("/knowledge-bases")
		{
			kbRoutes.GET("", deps.KnowledgeHandler.List)
			kbRoutes.POST("", deps.KnowledgeHandler.Create)
			kbRoutes.PUT("/:id", deps.KnowledgeHandler.Update)
			kbRoutes.DELETE("/:id", deps.KnowledgeHandler.Delete)
			kbRoutes.POST("/:id/files", deps.KnowledgeHandler.UploadFile)
			kbRoutes.GET("/:id/files", deps.KnowledgeHandler.ListFiles)
			kbRoutes.GET("/:id/search", deps.KnowledgeHandler.SearchFiles)
			kbRoutes.GET("/:id/files/:fileId/text", deps.KnowledgeHandler.GetFileText)
			kbRoutes.GET("/:id/files/:fileId/content", deps.KnowledgeHandler.GetFileContent)
			kbRoutes.DELETE("/:id/files/:fileId", deps.KnowledgeHandler.DeleteFile)
			kbRoutes.GET("/group/:groupId", deps.KnowledgeHandler.ListGroup)
			kbRoutes.GET("/resolve", deps.KnowledgeHandler.ResolveKnowledgeRef)
		}

		// Conversation routes
		convRoutes := apiGroup.Group("/conversations")
		convRoutes.Use(middleware.ValidateUUIDParam("id"), middleware.ValidateUUIDParam("messageId"))
		{
			convRoutes.POST("", deps.ConvHandler.Create)
			convRoutes.POST("/private", deps.ConvHandler.GetOrCreatePrivate)
			convRoutes.GET("", deps.ConvHandler.List)
			convRoutes.GET("/archived", deps.ConvHandler.ListArchived)
			convRoutes.PUT("/:id", deps.ConvHandler.RenameConversation)
			convRoutes.DELETE("/:id", deps.ConvHandler.Delete)
			convRoutes.POST("/:id/archive", deps.ConvHandler.ArchiveConversation)
			convRoutes.POST("/:id/unarchive", deps.ConvHandler.UnarchiveConversation)
			convRoutes.POST("/:id/pin", deps.ConvHandler.TogglePin)
			convRoutes.POST("/:id/messages", deps.MsgHandler.Send)
			convRoutes.GET("/:id/messages", deps.MsgHandler.History)
			convRoutes.GET("/:id/messages/search", deps.MsgHandler.Search)
			convRoutes.GET("/:id/pinned-context", deps.MsgHandler.PinnedContext)
			convRoutes.GET("/:id/blackboard", deps.MsgHandler.GetBlackboard)
			convRoutes.PUT("/:id/blackboard", deps.MsgHandler.UpdateBlackboard)
			convRoutes.PUT("/:id/read", deps.MsgHandler.MarkAsRead)
			convRoutes.GET("/:id/messages/unread", deps.MsgHandler.Unread)
			convRoutes.POST("/:id/messages/:messageId/pin", deps.MsgHandler.Pin)
			convRoutes.DELETE("/:id/messages/:messageId/pin", deps.MsgHandler.Unpin)
			convRoutes.DELETE("/:id/messages/:messageId", deps.MsgHandler.Recall)
			convRoutes.POST("/:id/messages/:messageId/hide", deps.MsgHandler.HideMessage)
			convRoutes.DELETE("/:id/messages/:messageId/hide", deps.MsgHandler.UnhideMessage)
			convRoutes.GET("/:id/messages/:messageId/replies", deps.MsgHandler.Replies)
			convRoutes.PATCH("/:id/messages/:messageId/cards", deps.MsgHandler.UpdateCard)
		}
		apiGroup.GET("/conversations/:id/agents", deps.ConvHandler.ListAgents)
		apiGroup.POST("/conversations/agent", deps.ConvHandler.GetOrCreateAgentPrivate)
		apiGroup.POST("/conversations/:id/agents", deps.ConvHandler.AddAgent)
		apiGroup.DELETE("/conversations/:id/agents/:agentID", deps.ConvHandler.RemoveAgent)
		apiGroup.PUT("/conversations/:id/agents/:agentID/role", deps.ConvHandler.SetAgentRole)

		// Agents
		apiGroup.GET("/agents", deps.AgentHandler.List)
		apiGroup.POST("/agents", deps.AgentHandler.Create)
		apiGroup.PUT("/agents/:id", deps.AgentHandler.Update)
		apiGroup.PUT("/agents/:id/avatar", deps.AgentHandler.UpdateAvatar)
		apiGroup.PUT("/agents/:id/tags", deps.AgentHandler.UpdateTags)
		apiGroup.PUT("/agents/:id/custom-skills", deps.AgentHandler.UpdateCustomSkills)
		apiGroup.DELETE("/agents/:id", deps.AgentHandler.Delete)
		apiGroup.POST("/agent-tokens", deps.AgentHandler.GenerateAgentToken)
		apiGroup.POST("/agents/:id/start", deps.AgentHandler.StartAgent)
		apiGroup.POST("/agents/:id/restart", deps.AgentHandler.RestartAgent)
		apiGroup.POST("/agents/:id/stop", deps.AgentHandler.StopAgent)
		apiGroup.POST("/agents/:id/skills/open-location", deps.AgentHandler.OpenSkillLocation)
		apiGroup.GET("/agents/:id/files/browse", deps.AgentHandler.BrowseFiles)

		// Platform skills
		apiGroup.GET("/platform-skills", deps.PlatformSkillHandler.List)
		apiGroup.POST("/platform-skills", deps.PlatformSkillHandler.Create)
		apiGroup.POST("/platform-skills/import-defaults", deps.PlatformSkillHandler.ImportDefaults)
		apiGroup.PUT("/platform-skills/:id", deps.PlatformSkillHandler.Update)
		apiGroup.DELETE("/platform-skills/:id", deps.PlatformSkillHandler.Delete)

		// Agent prompt templates
		apiGroup.GET("/agent-prompt-templates", deps.AgentPromptTemplateHandler.List)
		apiGroup.POST("/agent-prompt-templates", deps.AgentPromptTemplateHandler.Create)
		apiGroup.POST("/agent-prompt-templates/import-defaults", deps.AgentPromptTemplateHandler.ImportDefaults)
		apiGroup.PUT("/agent-prompt-templates/:id", deps.AgentPromptTemplateHandler.Update)
		apiGroup.DELETE("/agent-prompt-templates/:id", deps.AgentPromptTemplateHandler.Delete)

		// User templates
		apiGroup.GET("/user-templates", deps.UserTemplateHandler.List)
		apiGroup.POST("/user-templates", deps.UserTemplateHandler.Create)
		apiGroup.PUT("/user-templates/:id", deps.UserTemplateHandler.Update)
		apiGroup.DELETE("/user-templates/:id", deps.UserTemplateHandler.Delete)

		// Daemon machine management
		apiGroup.GET("/daemon/machines", deps.AgentHandler.ListDaemonMachines)
		apiGroup.POST("/daemon/machines", deps.AgentHandler.CreateDaemonMachine)
		apiGroup.DELETE("/daemon/machines/:id", deps.AgentHandler.DeleteDaemonMachine)
		apiGroup.GET("/daemon/machines/:id/connect", deps.AgentHandler.GetMachineConnect)
		apiGroup.GET("/daemon/agent-candidates", deps.AgentHandler.ListAgentCandidates)
		apiGroup.POST("/daemon/agent-candidates/:id/add", deps.AgentHandler.AddCandidateAgent)

		// Catalog (unified abstraction over platform_skill / tool_definition /
		// agent_prompt_template / user_template). Authenticated so the handler
		// can read user_id from JWT for user-scope domains; system-scope reads
		// work without any user_id.
		if deps.CatalogHandler != nil {
			catalogGroup := apiGroup.Group("/catalog")
			catalogGroup.GET("", deps.CatalogHandler.DomainsHandler)
			deps.CatalogHandler.Register(catalogGroup)
		}

		// Tasks
		taskRoutes := apiGroup.Group("/tasks")
		taskRoutes.Use(middleware.ValidateUUIDParam("id"))
		{
			taskRoutes.GET("", deps.TaskHandler.List)
			taskRoutes.POST("", deps.TaskHandler.Create)
			taskRoutes.PUT("/:id", deps.TaskHandler.Update)
			taskRoutes.POST("/:id/status", deps.TaskHandler.MoveStatus)
			taskRoutes.DELETE("/:id", deps.TaskHandler.Delete)
			taskRoutes.GET("/orch-cards", deps.TaskHandler.ListOrchCards)
		}

		// Artifacts
		artifactRoutes := apiGroup.Group("/artifacts")
		artifactRoutes.Use(middleware.ValidateUUIDParam("rootId"))
		{
			artifactRoutes.GET("/:rootId/versions", deps.ArtifactHandler.ListVersions)
			artifactRoutes.POST("/:rootId/versions", deps.ArtifactHandler.CreateVersion)
			artifactRoutes.POST("/:rootId/ai-edit", deps.ArtifactHandler.AIEdit)
			artifactRoutes.POST("/:rootId/deploy", deps.DeploymentHandler.Deploy)
			artifactRoutes.POST("/:rootId/deploy-github", deps.DeploymentHandler.DeployGitHub)
		}

		// Deployments
		apiGroup.GET("/deployments/capabilities", deps.DeploymentHandler.Capabilities)
		apiGroup.GET("/deployments/:id", middleware.ValidateUUIDParam("id"), deps.DeploymentHandler.Get)
		apiGroup.POST("/deployments/deploy", deps.DeploymentHandler.DeployByConversation)
	}

	// Public deployment routes (no JWT, auth via deployment ID)
	r.GET("/api/sites/:id/*filepath", middleware.ValidateUUIDParam("id"), deps.DeploymentHandler.ServeSite)
	r.GET("/api/deployments/:id/download", middleware.ValidateUUIDParam("id"), deps.DeploymentHandler.Download)
	r.GET("/api/deployments/:id/download/:name", middleware.ValidateUUIDParam("id"), deps.DeploymentHandler.Download)

	// Friends
	friendGroup := r.Group("/api/friends")
	friendGroup.Use(authMiddleware)
	{
		friendGroup.POST("/request", deps.FriendHandler.SendRequest)
		friendGroup.POST("/:id/accept", middleware.ValidateUUIDParam("id"), deps.FriendHandler.AcceptRequest)
		friendGroup.POST("/:id/reject", middleware.ValidateUUIDParam("id"), deps.FriendHandler.RejectRequest)
		friendGroup.GET("", deps.FriendHandler.ListFriends)
		friendGroup.GET("/pending", deps.FriendHandler.ListPending)
		friendGroup.GET("/search", deps.FriendHandler.SearchUsers)
		friendGroup.DELETE("/:id", middleware.ValidateUUIDParam("id"), deps.FriendHandler.DeleteFriend)
	}

	// Groups
	groupRoutes := r.Group("/api/groups")
	groupRoutes.Use(authMiddleware, middleware.ValidateUUIDParam("id"))
	{
		groupRoutes.POST("", deps.GroupHandler.CreateGroup)
		groupRoutes.GET("/:id", deps.GroupHandler.GetGroupInfo)
		groupRoutes.PUT("/:id", deps.GroupHandler.UpdateGroupInfo)
		groupRoutes.POST("/:id/members", deps.GroupHandler.AddMember)
		groupRoutes.DELETE("/:id/members/:userId", deps.GroupHandler.RemoveMember)
		groupRoutes.GET("/:id/members", deps.GroupHandler.ListMembers)
		groupRoutes.PUT("/:id/members/:memberId/role", deps.GroupHandler.ChangeMemberRole)
		groupRoutes.POST("/:id/leave", deps.GroupHandler.LeaveGroup)
		groupRoutes.POST("/:id/dissolve", deps.GroupHandler.DissolveGroup)
	}

	// Users
	userGroup := r.Group("/api/users")
	userGroup.Use(authMiddleware)
	{
		userGroup.GET("/search", deps.UserHandler.Search)
		userGroup.GET("/me", deps.UserHandler.GetProfile)
		userGroup.PUT("/me", deps.UserHandler.UpdateProfile)
	}

	// Daemon routes (machine-token auth)
	r.GET("/daemon/ws", deps.DaemonHandler.Handle)
	r.GET("/daemon/connect", deps.DaemonHandler.Handle)
	r.POST("/daemon/register", deps.DaemonHandler.WithMachine(deps.DaemonHandler.RegisterHTTP))
	r.GET("/daemon/agent-token", deps.DaemonHandler.WithMachine(deps.DaemonHandler.IssueAgentToken))
	r.GET("/daemon/tasks", deps.DaemonHandler.WithMachine(deps.DaemonHandler.ClaimTask))
	r.POST("/daemon/tasks/:id/complete", deps.DaemonHandler.WithMachine(deps.DaemonHandler.CompleteTask))
	r.POST("/daemon/tasks/:id/heartbeat", deps.DaemonHandler.WithMachine(deps.DaemonHandler.Heartbeat))

	// MCP routes (daemon-token auth, no JWT)
	mcpGroup := r.Group("/mcp")
	mcpGroup.Use(middleware.MCPAuth(deps.DaemonToken))
	{
		mcpGroup.GET("/conversations", deps.ConvHandler.List)
		mcpGroup.GET("/conversations/:id/agents", deps.ConvHandler.ListAgents)
		mcpGroup.GET("/conversations/:id/messages", deps.MsgHandler.History)

		mcpTaskRoutes := mcpGroup.Group("/tasks")
		mcpTaskRoutes.Use(middleware.ValidateUUIDParam("id"))
		{
			mcpTaskRoutes.GET("", deps.TaskHandler.List)
			mcpTaskRoutes.POST("", deps.TaskHandler.Create)
			mcpTaskRoutes.PUT("/:id", deps.TaskHandler.Update)
			mcpTaskRoutes.POST("/:id/status", deps.TaskHandler.MoveStatus)
			mcpTaskRoutes.DELETE("/:id", deps.TaskHandler.Delete)
		}

		mcpGroup.GET("/agents", deps.AgentHandler.MCPList)
		mcpGroup.POST("/agents", deps.AgentHandler.Create)
		mcpGroup.GET("/agents/:id", deps.AgentHandler.MCPGetAgentDetail)
		mcpGroup.PUT("/agents/:id", deps.AgentHandler.Update)
		mcpGroup.DELETE("/agents/:id", deps.AgentHandler.Delete)
		mcpGroup.POST("/agents/:id/start", deps.AgentHandler.StartAgent)
		mcpGroup.POST("/agents/:id/stop", deps.AgentHandler.StopAgent)
		mcpGroup.GET("/daemon/machines", deps.AgentHandler.ListDaemonMachines)
		mcpGroup.GET("/daemon/agent-candidates", deps.AgentHandler.ListAgentCandidates)
		mcpGroup.POST("/daemon/agent-candidates/:id/add", deps.AgentHandler.AddCandidateAgent)
		mcpGroup.POST("/groups", deps.GroupHandler.CreateGroup)
		mcpGroup.GET("/groups/:id", deps.GroupHandler.GetGroupInfo)
		mcpGroup.GET("/groups/:id/members", deps.GroupHandler.ListMembers)

		// Knowledge base (MCP)
		mcpGroup.GET("/knowledge-bases", deps.KnowledgeHandler.List)
		mcpGroup.GET("/knowledge-bases/:id/files", deps.KnowledgeHandler.ListFiles)
		mcpGroup.GET("/knowledge-bases/:id/search", deps.KnowledgeHandler.SearchFiles)
		mcpGroup.GET("/knowledge-bases/:id/files/:fileId/text", deps.KnowledgeHandler.GetFileText)

		// Platform skills (MCP)
		mcpGroup.GET("/platform-skills", deps.PlatformSkillHandler.List)
		mcpGroup.POST("/platform-skills/import-defaults", deps.PlatformSkillHandler.ImportDefaults)

		// Tool registry (MCP) - daemon fetches tool definitions at startup
		mcpGroup.GET("/tool-registry", deps.ToolDefHandler.ToolRegistry)
	}

	// Tool definitions and builtin templates (public, no auth)
	r.GET("/api/tools/definitions", deps.ToolDefHandler.ListDefinitions)
	r.GET("/api/tools/builtin-templates", deps.ToolDefHandler.ListBuiltinTemplates)
	r.GET("/api/tools/builtin-skill-templates", deps.ToolDefHandler.BuiltinSkillTemplates)
	r.GET("/api/tools/categories", deps.ToolCategoryHandler.List)
}

// makeUploadHandler returns a gin.HandlerFunc for serving authenticated upload files.
func makeUploadHandler(uploadDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		isThumbnail := strings.HasPrefix(filepath.ToSlash(filePath), "thumbnails/")
		ext := strings.ToLower(filepath.Ext(absPath))
		isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp"
		if isThumbnail || isImage {
			c.Header("Content-Disposition", "inline")
		} else {
			fileName := filepath.Base(absPath)
			c.Header("Content-Disposition", "attachment; filename=\""+fileName+"\"")
		}
		c.Header("X-Content-Type-Options", "nosniff")
		c.File(absPath)
	}
}

// cleanUploadRoutePath sanitizes the upload filepath parameter.
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
