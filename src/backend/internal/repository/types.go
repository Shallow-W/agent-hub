package repository

import (
	"context"
	"time"

	"github.com/agent-hub/backend/internal/model"
)

// ── Canonical Repository Interfaces ──────────────────────────────────────────
// These interfaces define the complete method set for each domain repository.
// New code should reference these interfaces rather than service-local subset
// interfaces defined in service/*.go.
//
// Each one is satisfied by the corresponding *XxxRepo struct defined in the
// sibling repository files (conversation.go, agent.go, etc.).

// MessageStore is the canonical interface for message persistence.
// Satisfied by *MessageRepo.
type MessageStore interface {
	Create(ctx context.Context, conversationID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, senderID *string, mentions []string) (*model.Message, error)
	ListByConversation(ctx context.Context, conversationID string, before interface{}, limit int) ([]model.Message, error)
	MarkConversationRead(ctx context.Context, conversationID, userID string) error
	GetMessagesAfter(ctx context.Context, conversationID string, afterTime interface{}, limit int) ([]model.Message, error)
	GetByID(ctx context.Context, id string) (*model.Message, error)
	GetMessageSender(ctx context.Context, messageID string) (string, error)
	SearchByContent(ctx context.Context, conversationID, keyword string, limit int) ([]model.Message, error)
	SoftDelete(ctx context.Context, messageID string) error
	SaveArtifacts(ctx context.Context, messageID string, artifacts []model.Artifact) error
	UpdateMessageCards(ctx context.Context, messageID, cardsJSON string) error
	PinMessage(ctx context.Context, conversationID, messageID, userID string) (*model.MessagePin, error)
	UnpinMessage(ctx context.Context, conversationID, messageID string) error
	ListPinnedMessages(ctx context.Context, conversationID string, limit int) ([]model.PinnedMessage, error)
	GetConversationBlackboard(ctx context.Context, conversationID string) (*model.ConversationBlackboard, error)
	UpsertConversationBlackboard(ctx context.Context, conversationID, manualContext, userID string) (*model.ConversationBlackboard, error)
	ListReplies(ctx context.Context, messageID string) ([]model.Message, error)
	HideMessage(ctx context.Context, userID, messageID string) error
	UnhideMessage(ctx context.Context, userID, messageID string) error
	GetHiddenMessageIDs(ctx context.Context, userID, conversationID string) (map[string]bool, error)
	CreateStreaming(ctx context.Context, conversationID, role string, senderID *string, replyTo *string, artifactsJSON string) (*model.Message, error)
	FinalizeStreaming(ctx context.Context, messageID, status, content, blocksJSON, artifactsJSON string) error
	ListStreaming(ctx context.Context) ([]model.Message, error)
	ListStaleStreaming(ctx context.Context, before time.Time) ([]model.Message, error)
	MarkStaleStreaming(ctx context.Context, maxAge time.Duration) (int, error)
}

// ConvStore is the canonical interface for conversation persistence.
// Satisfied by *ConversationRepo.
type ConvStore interface {
	Create(ctx context.Context, userID, convType, title string) (*model.Conversation, error)
	ListByUserID(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, error)
	ListArchivedByUserID(ctx context.Context, userID string, limit, offset int) ([]model.Conversation, error)
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	Delete(ctx context.Context, id string) error
	UpdatePinned(ctx context.Context, id string, pinned bool) error
	UpdateTimestamp(ctx context.Context, id string) error
	UpdateTitle(ctx context.Context, id, title string) error
	Archive(ctx context.Context, id string) error
	Unarchive(ctx context.Context, id string) error
	ArchiveForMember(ctx context.Context, conversationID, userID string) error
	UnarchiveForMember(ctx context.Context, conversationID, userID string) error
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	DeleteMember(ctx context.Context, conversationID, userID string) error
	AddMember(ctx context.Context, conversationID, userID, role string) error
	FindPrivateChat(ctx context.Context, userID, friendID string) (*model.Conversation, error)
	CreatePrivateChat(ctx context.Context, userID, friendID, title string) (*model.Conversation, error)
	FindAgentChat(ctx context.Context, userID, agentID string) (*model.Conversation, error)
	CreateAgentChat(ctx context.Context, userID, agentID string) (*model.Conversation, error)
	ListAgents(ctx context.Context, conversationID, userID string) ([]model.ConversationAgent, error)
	AddAgent(ctx context.Context, conversationID, agentID, userID string) (*model.ConversationAgent, error)
	RemoveAgent(ctx context.Context, conversationID, agentID, userID string) (bool, error)
	UpdateAgentRole(ctx context.Context, conversationID, agentID, role string) error
	GetOrchestrator(ctx context.Context, conversationID string) (*model.ConversationAgent, error)
	ListMemberIDs(ctx context.Context, conversationID string) ([]string, error)
}

// AgentStore is the canonical interface for agent persistence.
// Satisfied by *AgentRepo.
type AgentStore interface {
	ListAvailable(ctx context.Context, userID string) ([]model.Agent, error)
	GetByID(ctx context.Context, id string) (*model.Agent, error)
	GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error)
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error)
	ClaimDaemonTask(ctx context.Context, machineID string) (*model.DaemonTask, error)
	CompleteDaemonTask(ctx context.Context, id, machineID, result, taskError string) (bool, error)
	UpsertSystemAgent(ctx context.Context, name, cliTool, version, capabilitiesJSON, machineID string) error
	CreateDaemonMachine(ctx context.Context, userID, name, apiKeyHash string) (*model.DaemonMachine, error)
	ListDaemonMachines(ctx context.Context, userID string) ([]model.DaemonMachine, error)
	DeleteDaemonMachine(ctx context.Context, id, userID string) (bool, error)
	GetDaemonMachineByAPIKeyHash(ctx context.Context, apiKeyHash string) (*model.DaemonMachine, error)
	GetDaemonMachineByID(ctx context.Context, id string) (*model.DaemonMachine, error)
	GetAgentsByMachine(ctx context.Context, machineID string) ([]model.Agent, error)
	MarkDaemonMachineConnected(ctx context.Context, id, machineID string) error
	UpdateMachineCapabilities(ctx context.Context, id string, capabilities []string) error
	FindMachineWithCapability(ctx context.Context, userID, capability string) (*model.DaemonMachine, error)
	UpsertMachineAgentCandidate(ctx context.Context, machineID, name, cliTool, version, capabilitiesJSON string) error
	ListAgentCandidates(ctx context.Context, userID string) ([]model.AgentCandidate, error)
	AddCandidateAgent(ctx context.Context, userID, candidateID, displayName, expectedCLITool, systemPrompt, toolsConfig, customSkills string, enableManagementTools bool) (*model.Agent, error)
	CreateCustom(ctx context.Context, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, customSkills string, enableManagementTools bool) (*model.Agent, error)
	UpdateCustom(ctx context.Context, id, userID, name, cliTool, systemPrompt, toolsConfig, avatar, capabilitiesJSON, customSkills string, enableManagementTools bool) (*model.Agent, error)
	UpdateAvatar(ctx context.Context, id, userID, avatar string) (*model.Agent, error)
	UpdateTags(ctx context.Context, id, tags string) (*model.Agent, error)
	UpdateCustomSkills(ctx context.Context, id, userID, customSkills string) (*model.Agent, error)
	UpdateAgentStatus(ctx context.Context, id, status string) error
	ClearAgentMachine(ctx context.Context, id string) error
	MarkMachineAgentsStopped(ctx context.Context, machineID string) error
	UpdateMachineAPIKey(ctx context.Context, id, apiKeyHash string) error
	DeleteOwned(ctx context.Context, id, userID string) (bool, error)
	IsAgentInConversation(ctx context.Context, conversationID, agentID, userID string) (bool, error)
	SetDaemonTaskOrch(ctx context.Context, taskID, orchTaskID, workerName string)
}

// OrchTaskStore is the canonical interface for orchestration task persistence.
// Satisfied by *OrchTaskRepo.
type OrchTaskStoreCanon interface {
	Create(ctx context.Context, task *model.OrchTask) error
	GetByID(ctx context.Context, id string) (*model.OrchTask, error)
	UpdateStatus(ctx context.Context, id, status string) error
	UpdateDispatchMessageID(ctx context.Context, id, messageID string) error
	UpdateStatusCAS(ctx context.Context, id, fromStatus, toStatus string) (bool, error)
	UpdateWorkerResult(ctx context.Context, id, workerName, status, result string) (bool, error)
	SetSummaryAndEvaluate(ctx context.Context, id, summary string) error
	IncrementRound(ctx context.Context, id string) error
}

// DeploymentStore is the canonical interface for deployment persistence.
// Satisfied by *DeploymentRepo.
type DeploymentStore interface {
	Create(ctx context.Context, d model.Deployment) (*model.Deployment, error)
	UpdateStatus(ctx context.Context, id, status, url, errMsg string) (*model.Deployment, error)
	GetByID(ctx context.Context, id string) (*model.Deployment, error)
}

// ArtifactStore is the canonical interface for artifact persistence.
// Satisfied by *ArtifactRepo.
type ArtifactStore interface {
	ListVersions(ctx context.Context, rootID string) ([]model.Artifact, error)
	CreateVersion(ctx context.Context, rootID string, in model.Artifact) (*model.Artifact, error)
	GetConversationIDByRoot(ctx context.Context, rootID string) (string, error)
	GetLatestByRoot(ctx context.Context, rootID string) (*model.Artifact, error)
	GetLatestRootByConversation(ctx context.Context, convID string) (string, error)
	GetLatestByConversationAndName(ctx context.Context, convID, name string) (*model.Artifact, error)
}

// KnowledgeStore is the canonical interface for knowledge base persistence.
// Satisfied by *KnowledgeRepo.
type KnowledgeStore interface {
	Create(ctx context.Context, userID, name, description string) (*model.KnowledgeBase, error)
	ListByUser(ctx context.Context, userID string) ([]model.KnowledgeBase, error)
	GetByID(ctx context.Context, id string) (*model.KnowledgeBase, error)
	UpdateVisibility(ctx context.Context, id, visibility string) error
	Delete(ctx context.Context, id string) error
	ListFiles(ctx context.Context, kbID string) ([]model.KnowledgeFile, error)
	AddFile(ctx context.Context, kbID, filename, filePath string, fileSize int64, mimeType, sha256, previewText, previewType string) (*model.KnowledgeFile, error)
	DeleteFile(ctx context.Context, kbID, fileID string) (string, error)
	FindPublicByName(ctx context.Context, username, kbName string) (*model.KnowledgeBase, error)
	FindByUserAndName(ctx context.Context, userID, kbName string) (*model.KnowledgeBase, error)
	GetFileByID(ctx context.Context, kbID, fileID string) (*model.KnowledgeFile, error)
	UpdateFilePreview(ctx context.Context, kbID, fileID, previewText, previewType string) error
	UpdateFileName(ctx context.Context, kbID, fileID, filename string) (*model.KnowledgeFile, error)
	SearchFiles(ctx context.Context, kbID, keyword string, limit int) ([]model.KnowledgeFile, error)
	ListPublicByUsers(ctx context.Context, userIDs []string, excludeUserID string) ([]model.KnowledgeBase, error)
}
