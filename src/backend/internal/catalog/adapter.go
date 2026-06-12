package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-hub/backend/internal/model"
)

// ── Narrow repo interfaces ───────────────────────────────────────────────────
//
// We deliberately declare these locally in the catalog package (mirroring the
// pattern in service/platform_skill.go etc.) instead of importing concrete
// *repository.XxxRepo structs. The existing repository implementations already
// satisfy them. This keeps catalog -> repository a one-way dependency and the
// adapter unit-testable with fakes.

type PlatformSkillStore interface {
	ListByUser(ctx context.Context, userID string) ([]model.PlatformSkill, error)
	Create(ctx context.Context, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error)
	Update(ctx context.Context, id, userID, name, category, description, trigger, detail string) (*model.PlatformSkill, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

type ToolDefinitionLister interface {
	List(ctx context.Context) ([]model.ToolDefinition, error)
}

type AgentPromptTemplateStore interface {
	ListByUser(ctx context.Context, userID string) ([]model.AgentPromptTemplate, error)
	Create(ctx context.Context, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error)
	Update(ctx context.Context, id, userID, name, category, description, systemPrompt string) (*model.AgentPromptTemplate, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

type UserTemplateStore interface {
	ListByUserAndType(ctx context.Context, userID, tplType string) ([]model.UserTemplate, error)
	Create(ctx context.Context, userID, tplType, name, content string) (*model.UserTemplate, error)
	Update(ctx context.Context, id, userID, name, content string) (*model.UserTemplate, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

// AdapterStore implements Store by delegating to DomainPlugins. It performs the
// model <-> Item conversion at the boundary so the rest of the catalog package
// sees only the unified shape.
//
// To add a new catalog domain: implement DomainPlugin, register it via
// AdapterDeps.Plugins, and add a DomainSpec to the Registry. You do NOT need
// to modify this file.
type AdapterStore struct {
	plugins  map[Domain]DomainPlugin
	registry *Registry
	// toolDefRepo is retained solely for GetByID, which scans by name.
	toolDefRepo ToolDefinitionLister
}

// AdapterDeps bundles the plugin map and registry. Fields may be nil if the
// caller doesn't yet need that domain — List on an unregistered domain returns
// ErrUnknownDomain via the registry lookup.
type AdapterDeps struct {
	Plugins  map[Domain]DomainPlugin
	Registry *Registry
}

// NewAdapterStore builds an AdapterStore from the given deps. The Registry
// is consulted for every operation; only domains that appear in the registry
// are reachable.
func NewAdapterStore(deps AdapterDeps) *AdapterStore {
	// Extract toolDefRepo for GetByID support.
	var toolDefRepo ToolDefinitionLister
	if td, ok := deps.Plugins[DomainToolDefinition]; ok {
		if p, ok := td.(*toolDefinitionPlugin); ok {
			toolDefRepo = p.repo
		}
	}
	return &AdapterStore{
		plugins:     deps.Plugins,
		registry:    deps.Registry,
		toolDefRepo: toolDefRepo,
	}
}

// Registry exposes the spec registry so Service / Handler can re-use the
// same registration data without holding a second copy.
func (s *AdapterStore) Registry() *Registry { return s.registry }

// plugin looks up the DomainPlugin for a given domain.
func (s *AdapterStore) plugin(d Domain) (DomainPlugin, error) {
	p, ok := s.plugins[d]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownDomain, d)
	}
	return p, nil
}

// requireDomain returns the spec for d, or ErrUnknownDomain if it isn't
// registered.
func (s *AdapterStore) requireDomain(d Domain) (DomainSpec, error) {
	if s.registry == nil {
		return DomainSpec{}, ErrUnknownDomain
	}
	spec, ok := s.registry.Get(d)
	if !ok {
		return DomainSpec{}, fmt.Errorf("%w: %s", ErrUnknownDomain, d)
	}
	return spec, nil
}

// List dispatches by domain. For system-scope domains the UserID filter is
// ignored. For user_template, the Subtype filter is handled specially.
func (s *AdapterStore) List(ctx context.Context, domain Domain, q ListQuery) ([]Item, error) {
	if _, err := s.requireDomain(domain); err != nil {
		return nil, err
	}

	// user_template has subtype-aware listing — handle it specially.
	if domain == DomainUserTemplate {
		p, err := s.plugin(domain)
		if err != nil {
			return nil, err
		}
		utp, ok := p.(*userTemplatePlugin)
		if !ok {
			// Fallback: call generic List (loses subtype filter).
			return p.List(ctx, q.UserID)
		}
		return utp.ListWithSubtype(ctx, q.UserID, q.Subtype)
	}

	p, err := s.plugin(domain)
	if err != nil {
		return nil, err
	}
	return p.List(ctx, q.UserID)
}

// ── Read paths: GetByID ──────────────────────────────────────────────────────

// GetByID retrieves a single item by its ID. Currently this only supports the
// tool_definition domain (system scope) because user-scope domains require a
// userID to efficiently locate items, and GetByID does not carry one.
//
// For platform_skill, agent_prompt_template, and user_template, this method
// returns ErrNotFound. The catalog handler's GET-by-ID endpoint will therefore
// return 404 for those domains — this is acceptable because the frontend uses
// list endpoints (which do carry userID), not get-by-ID, for user-scope items.
func (s *AdapterStore) GetByID(ctx context.Context, id string) (*Item, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrInvalid
	}
	if s.toolDefRepo != nil {
		item, err := toolDefFindByID(s.toolDefRepo, ctx, id)
		if err != nil {
			return nil, err
		}
		if item != nil {
			return item, nil
		}
	}
	return nil, ErrNotFound
}

// ── Write paths ──────────────────────────────────────────────────────────────

func (s *AdapterStore) Create(ctx context.Context, input CreateInput) (*Item, error) {
	if _, err := s.requireDomain(input.Domain); err != nil {
		return nil, err
	}
	p, err := s.plugin(input.Domain)
	if err != nil {
		return nil, err
	}
	return p.Create(ctx, input)
}

func (s *AdapterStore) Update(ctx context.Context, id string, input UpdateInput) (*Item, error) {
	if _, err := s.requireDomain(input.Domain); err != nil {
		return nil, err
	}
	p, err := s.plugin(input.Domain)
	if err != nil {
		return nil, err
	}
	return p.Update(ctx, id, input.UserID, input)
}

func (s *AdapterStore) Delete(ctx context.Context, domain Domain, userID, id string) error {
	if _, err := s.requireDomain(domain); err != nil {
		return err
	}
	p, err := s.plugin(domain)
	if err != nil {
		return err
	}
	return p.Delete(ctx, id, userID)
}

// decodePlatformSkillPayload parses the JSON payload carried by Create /
// Update for the platform_skill domain into the (trigger, detail) pair the
// underlying repo expects. An empty payload yields two empty strings. A
// malformed payload yields ErrInvalid.
func decodePlatformSkillPayload(raw string) (trigger, detail string, err error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", nil
	}
	var m map[string]string
	if e := json.Unmarshal([]byte(raw), &m); e != nil {
		return "", "", fmt.Errorf("%w: payload: %v", ErrInvalid, e)
	}
	return m["trigger"], m["detail"], nil
}

func decodeAgentPromptPayload(raw string) (systemPrompt string, err error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	var m map[string]string
	if e := json.Unmarshal([]byte(raw), &m); e != nil {
		return "", fmt.Errorf("%w: payload: %v", ErrInvalid, e)
	}
	return m["system_prompt"], nil
}

// ── model -> Item converters ──────────────────────────────────────────────────

func platformSkillToItem(m *model.PlatformSkill) Item {
	payload := map[string]string{
		"trigger": m.Trigger,
		"detail":  m.Detail,
	}
	return Item{
		ID:          m.ID,
		Domain:      DomainPlatformSkill,
		UserID:      strPtr(m.UserID),
		Key:         m.Name,
		Label:       m.Name,
		Category:    m.Category,
		Description: m.Description,
		PayloadJSON: mustJSON(payload),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func toolDefinitionToItem(m *model.ToolDefinition) Item {
	return Item{
		ID:          m.Name,
		Domain:      DomainToolDefinition,
		UserID:      nil,
		Key:         m.Name,
		Label:       firstNonEmpty(m.Label, m.Name),
		Category:    m.Category,
		Description: m.Description,
		PayloadJSON: "",
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.CreatedAt,
	}
}

func agentPromptTemplateToItem(m *model.AgentPromptTemplate) Item {
	payload := map[string]string{
		"system_prompt": m.SystemPrompt,
	}
	return Item{
		ID:          m.ID,
		Domain:      DomainAgentPromptTemplate,
		UserID:      strPtr(m.UserID),
		Key:         m.Name,
		Label:       m.Name,
		Category:    m.Category,
		Description: m.Description,
		PayloadJSON: mustJSON(payload),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func userTemplateToItem(m *model.UserTemplate) Item {
	return Item{
		ID:          m.ID,
		Domain:      DomainUserTemplate,
		UserID:      strPtr(m.UserID),
		Subtype:     m.Type,
		Key:         m.Name,
		Label:       m.Name,
		PayloadJSON: string(m.Content),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// ── small helpers ────────────────────────────────────────────────────────────

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
