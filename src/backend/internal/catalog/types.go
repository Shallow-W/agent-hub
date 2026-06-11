// Package catalog provides a unified abstraction over the four parallel
// "directory" vertical slices currently scattered across
// platform_skill / tool_definition / agent_prompt_template /
// user_template services and repos.
//
// The goal is NOT to physically merge the four DB tables (that's a later
// migration), but to consolidate their CRUD code skeleton. Adding a new
// catalog domain should only require registering a DomainSpec and wiring
// an adapter — not writing a whole new handler/service/repo/model/migration.
//
// Files in this package:
//   - types.go     — shared types (Domain, Scope, Item, inputs) + errors
//   - store.go     — Store interface (capability description only)
//   - registry.go  — DomainSpec + Registry (extensibility core)
//   - adapter.go   — AdapterStore that proxies to the 4 existing repos
//   - service.go   — Service (normalize / error mapping / defaults seeding)
//   - handler.go   — unified REST handler mounted under /api/catalog/:domain
//   - domains.go   — DefaultRegistry that registers all 4 known domains
package catalog

import (
	"errors"
	"time"
)

// Domain identifies a catalog domain. New domains can be added by
// registering a DomainSpec; the catalog core never hard-codes a switch
// over these values.
type Domain string

const (
	DomainPlatformSkill       Domain = "platform_skill"
	DomainToolDefinition      Domain = "tool_definition"
	DomainAgentPromptTemplate Domain = "agent_prompt_template"
	DomainUserTemplate        Domain = "user_template"
)

// Scope tells whether a domain is per-user (CRUD) or system-wide (read-only).
type Scope string

const (
	ScopeSystem Scope = "system"
	ScopeUser   Scope = "user"
)

// Item is the canonical, domain-agnostic representation of one catalog row.
// Each existing model (model.PlatformSkill, model.ToolDefinition, ...)
// round-trips into this shape; consumers that need domain-specific fields
// unmarshal PayloadJSON themselves.
type Item struct {
	ID          string    `json:"id"`
	Domain      Domain    `json:"domain"`
	UserID      *string   `json:"user_id,omitempty"` // nil for system scope
	Subtype     string    `json:"subtype,omitempty"` // "" if domain has no subtype
	Key         string    `json:"key"`               // unique within (domain, user_id, subtype)
	Category    string    `json:"category,omitempty"`
	Label       string    `json:"label"`
	Description string    `json:"description,omitempty"`
	PayloadJSON string    `json:"payload,omitempty"` // domain-specific structured payload
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListQuery carries optional filters to List. Fields with zero values are
// ignored.
type ListQuery struct {
	UserID   string
	Subtype  string // optional
	Category string // optional
}

// CreateInput is the normalized write payload for Create. Domain and UserID
// are required for user-scope domains; Subtype/PayloadJSON depend on the
// DomainSpec.
type CreateInput struct {
	Domain      Domain
	UserID      string // ignored for system scope
	Subtype     string
	Key         string
	Category    string
	Label       string
	Description string
	PayloadJSON string
}

// UpdateInput is the partial-update payload. Pointer fields are optional;
// a nil pointer leaves the existing value untouched.
//
// Domain tells the Store which repo to dispatch to (mirrors CreateInput).
// UserID is required for user-scope domains (the repo enforces it on every
// write to prevent cross-user mutation). System-scope domains ignore it.
type UpdateInput struct {
	Domain      Domain
	UserID      string
	Key         *string
	Category    *string
	Label       *string
	Description *string
	PayloadJSON *string
}

// Unified error sentinels. Service implementations should map storage-level
// errors (sql.ErrNoRows, unique violation, ...) onto these.
var (
	ErrNotFound      = errors.New("catalog: item not found")
	ErrInvalid       = errors.New("catalog: invalid input")
	ErrDuplicate     = errors.New("catalog: duplicate key")
	ErrUnknownDomain = errors.New("catalog: unknown domain")
	ErrReadOnly      = errors.New("catalog: domain is read-only")
)
