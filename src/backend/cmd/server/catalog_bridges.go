package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/agent-hub/backend/internal/catalog"
	"github.com/agent-hub/backend/internal/service"
)

// ---------------------------------------------------------------------------
// Generic bridge infrastructure
// ---------------------------------------------------------------------------

// bridgeConfig describes how to adapt catalog.Service to a domain-specific
// typed interface. Each domain provides: error mapping and item-to-DTO conversion.
type bridgeConfig[DTO any] struct {
	Domain    catalog.Domain
	MapErr    func(error) error
	ItemToDTO func(catalog.Item, string) DTO
}

// bridgeCRUD provides generic CRUD dispatch for a bridgeConfig.
// Domain-specific bridge structs embed this and only add typed method signatures.
type bridgeCRUD[DTO any] struct {
	svc *catalog.Service
	cfg bridgeConfig[DTO]
}

func (b bridgeCRUD[DTO]) list(ctx context.Context, userID, subtype string) ([]DTO, error) {
	items, err := b.svc.List(ctx, b.cfg.Domain, catalog.ListQuery{UserID: userID, Subtype: subtype})
	if err != nil {
		return nil, b.cfg.MapErr(err)
	}
	out := make([]DTO, 0, len(items))
	for _, it := range items {
		out = append(out, b.cfg.ItemToDTO(it, userID))
	}
	return out, nil
}

func (b bridgeCRUD[DTO]) create(ctx context.Context, input catalog.CreateInput) (*DTO, error) {
	item, err := b.svc.Create(ctx, input)
	if err != nil {
		return nil, b.cfg.MapErr(err)
	}
	if item == nil {
		return nil, b.cfg.MapErr(catalog.ErrNotFound)
	}
	out := b.cfg.ItemToDTO(*item, input.UserID)
	return &out, nil
}

func (b bridgeCRUD[DTO]) update(ctx context.Context, id string, input catalog.UpdateInput) (*DTO, error) {
	item, err := b.svc.Update(ctx, id, input)
	if err != nil {
		return nil, b.cfg.MapErr(err)
	}
	if item == nil {
		return nil, b.cfg.MapErr(catalog.ErrNotFound)
	}
	out := b.cfg.ItemToDTO(*item, input.UserID)
	return &out, nil
}

func (b bridgeCRUD[DTO]) delete(ctx context.Context, userID, id string) error {
	if err := b.svc.Delete(ctx, b.cfg.Domain, userID, id); err != nil {
		return b.cfg.MapErr(err)
	}
	return nil
}

// mapCatalogErr creates a domain-specific error mapper from three sentinel
// errors. This replaces the three near-identical mapCatalogToXxxErr functions
// that were previously inline in main.go.
func mapCatalogErr(notFound, duplicate, invalid error) func(error) error {
	return func(err error) error {
		if err == nil {
			return nil
		}
		switch {
		case errors.Is(err, catalog.ErrNotFound):
			return notFound
		case errors.Is(err, catalog.ErrDuplicate):
			return duplicate
		case errors.Is(err, catalog.ErrInvalid):
			return invalid
		default:
			return err
		}
	}
}

// ---------------------------------------------------------------------------
// Shared payload helpers
// ---------------------------------------------------------------------------

// mustJSONMap serialises a string map as JSON. Falls back to "{}" on error.
func mustJSONMap(m map[string]string) string {
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// decodePayloadString extracts a single key from a JSON string payload.
// Returns "" for empty or malformed payloads.
func decodePayloadString(raw, key string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	return m[key]
}

// ---------------------------------------------------------------------------
// Tool Definition Bridge (list-only)
// ---------------------------------------------------------------------------

// toolDefinitionCatalogBridge adapts *catalog.Service to the
// service.ToolDefinitionCatalogLister interface declared in the service package.
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

// ---------------------------------------------------------------------------
// Platform Skill Bridge
// ---------------------------------------------------------------------------

type platformSkillCatalogBridge struct {
	bridgeCRUD[service.PlatformSkillCatalogItem]
}

func newPlatformSkillBridge(svc *catalog.Service) platformSkillCatalogBridge {
	return platformSkillCatalogBridge{
		bridgeCRUD: bridgeCRUD[service.PlatformSkillCatalogItem]{
			svc: svc,
			cfg: bridgeConfig[service.PlatformSkillCatalogItem]{
				Domain: catalog.DomainPlatformSkill,
				MapErr: mapCatalogErr(
					service.ErrPlatformSkillNotFound,
					service.ErrPlatformSkillDuplicate,
					service.ErrPlatformSkillInvalid,
				),
				ItemToDTO: platformSkillItemFromCatalog,
			},
		},
	}
}

func (b platformSkillCatalogBridge) ListPlatformSkills(ctx context.Context, userID string) ([]service.PlatformSkillCatalogItem, error) {
	return b.list(ctx, userID, "")
}

func (b platformSkillCatalogBridge) CreatePlatformSkill(ctx context.Context, userID, name, category, description, trigger, detail string) (*service.PlatformSkillCatalogItem, error) {
	return b.create(ctx, catalog.CreateInput{
		Domain:      catalog.DomainPlatformSkill,
		UserID:      userID,
		Key:         name,
		Label:       name,
		Category:    category,
		Description: description,
		PayloadJSON: mustJSONMap(map[string]string{"trigger": trigger, "detail": detail}),
	})
}

func (b platformSkillCatalogBridge) UpdatePlatformSkill(ctx context.Context, id, userID, name, category, description, trigger, detail string) (*service.PlatformSkillCatalogItem, error) {
	key := name
	label := name
	payload := mustJSONMap(map[string]string{"trigger": trigger, "detail": detail})
	return b.update(ctx, id, catalog.UpdateInput{
		Domain:      catalog.DomainPlatformSkill,
		UserID:      userID,
		Key:         &key,
		Label:       &label,
		Category:    &category,
		Description: &description,
		PayloadJSON: &payload,
	})
}

func (b platformSkillCatalogBridge) DeletePlatformSkill(ctx context.Context, id, userID string) error {
	return b.delete(ctx, userID, id)
}

func platformSkillItemFromCatalog(it catalog.Item, userID string) service.PlatformSkillCatalogItem {
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
		Trigger:     decodePayloadString(it.PayloadJSON, "trigger"),
		Detail:      decodePayloadString(it.PayloadJSON, "detail"),
		CreatedAt:   it.CreatedAt,
		UpdatedAt:   it.UpdatedAt,
	}
}

// ---------------------------------------------------------------------------
// Agent Prompt Template Bridge
// ---------------------------------------------------------------------------

type agentPromptCatalogBridge struct {
	bridgeCRUD[service.AgentPromptTemplateCatalogItem]
}

func newAgentPromptBridge(svc *catalog.Service) agentPromptCatalogBridge {
	return agentPromptCatalogBridge{
		bridgeCRUD: bridgeCRUD[service.AgentPromptTemplateCatalogItem]{
			svc: svc,
			cfg: bridgeConfig[service.AgentPromptTemplateCatalogItem]{
				Domain: catalog.DomainAgentPromptTemplate,
				MapErr: mapCatalogErr(
					service.ErrAgentPromptTemplateNotFound,
					service.ErrAgentPromptTemplateDuplicate,
					service.ErrAgentPromptTemplateInvalid,
				),
				ItemToDTO: agentPromptItemFromCatalog,
			},
		},
	}
}

func (b agentPromptCatalogBridge) ListAgentPromptTemplates(ctx context.Context, userID string) ([]service.AgentPromptTemplateCatalogItem, error) {
	return b.list(ctx, userID, "")
}

func (b agentPromptCatalogBridge) CreateAgentPromptTemplate(ctx context.Context, userID, name, category, description, systemPrompt string) (*service.AgentPromptTemplateCatalogItem, error) {
	return b.create(ctx, catalog.CreateInput{
		Domain:      catalog.DomainAgentPromptTemplate,
		UserID:      userID,
		Key:         name,
		Label:       name,
		Category:    category,
		Description: description,
		PayloadJSON: mustJSONMap(map[string]string{"system_prompt": systemPrompt}),
	})
}

func (b agentPromptCatalogBridge) UpdateAgentPromptTemplate(ctx context.Context, id, userID, name, category, description, systemPrompt string) (*service.AgentPromptTemplateCatalogItem, error) {
	key := name
	label := name
	payload := mustJSONMap(map[string]string{"system_prompt": systemPrompt})
	return b.update(ctx, id, catalog.UpdateInput{
		Domain:      catalog.DomainAgentPromptTemplate,
		UserID:      userID,
		Key:         &key,
		Label:       &label,
		Category:    &category,
		Description: &description,
		PayloadJSON: &payload,
	})
}

func (b agentPromptCatalogBridge) DeleteAgentPromptTemplate(ctx context.Context, id, userID string) error {
	return b.delete(ctx, userID, id)
}

func agentPromptItemFromCatalog(it catalog.Item, userID string) service.AgentPromptTemplateCatalogItem {
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
		SystemPrompt: decodePayloadString(it.PayloadJSON, "system_prompt"),
		CreatedAt:    it.CreatedAt,
		UpdatedAt:    it.UpdatedAt,
	}
}

// ---------------------------------------------------------------------------
// User Template Bridge
// ---------------------------------------------------------------------------

type userTemplateCatalogBridge struct {
	bridgeCRUD[service.UserTemplateCatalogItem]
}

func newUserTemplateBridge(svc *catalog.Service) userTemplateCatalogBridge {
	return userTemplateCatalogBridge{
		bridgeCRUD: bridgeCRUD[service.UserTemplateCatalogItem]{
			svc: svc,
			cfg: bridgeConfig[service.UserTemplateCatalogItem]{
				Domain: catalog.DomainUserTemplate,
				MapErr: mapCatalogErr(
					service.ErrUserTemplateNotFound,
					service.ErrUserTemplateDuplicate,
					service.ErrUserTemplateInvalid,
				),
				ItemToDTO: userTemplateItemFromCatalog,
			},
		},
	}
}

func (b userTemplateCatalogBridge) ListUserTemplates(ctx context.Context, userID, tplType string) ([]service.UserTemplateCatalogItem, error) {
	return b.list(ctx, userID, tplType)
}

func (b userTemplateCatalogBridge) CreateUserTemplate(ctx context.Context, userID, tplType, name, content string) (*service.UserTemplateCatalogItem, error) {
	return b.create(ctx, catalog.CreateInput{
		Domain:      catalog.DomainUserTemplate,
		UserID:      userID,
		Subtype:     tplType,
		Key:         name,
		Label:       name,
		PayloadJSON: content,
	})
}

func (b userTemplateCatalogBridge) UpdateUserTemplate(ctx context.Context, id, userID, name, content string) (*service.UserTemplateCatalogItem, error) {
	key := name
	label := name
	payload := content
	return b.update(ctx, id, catalog.UpdateInput{
		Domain:      catalog.DomainUserTemplate,
		UserID:      userID,
		Key:         &key,
		Label:       &label,
		PayloadJSON: &payload,
	})
}

func (b userTemplateCatalogBridge) DeleteUserTemplate(ctx context.Context, id, userID string) error {
	return b.delete(ctx, userID, id)
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

// Compile-time assertions that bridge types satisfy their service interfaces.
var (
	_ service.ToolDefinitionCatalogLister     = toolDefinitionCatalogBridge{}
	_ service.PlatformSkillCatalogStore       = platformSkillCatalogBridge{}
	_ service.AgentPromptTemplateCatalogStore = agentPromptCatalogBridge{}
	_ service.UserTemplateCatalogStore        = userTemplateCatalogBridge{}
)
