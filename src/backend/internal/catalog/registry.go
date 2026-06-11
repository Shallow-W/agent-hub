package catalog

// DomainSpec describes how a specific catalog domain behaves. All
// domain-specific differences (scope, allowed subtypes, length limits,
// default category, default-value seeder) live here as data, so the
// catalog core (service / handler) never needs to switch on domain.
//
// To add a new catalog domain: register a DomainSpec via NewRegistry
// (typically in domains.go) and supply an adapter that knows how to
// convert to/from Item. You do NOT need to touch service.go or handler.go.
type DomainSpec struct {
	Name            Domain
	Label           string
	Scope           Scope
	Subtypes        []string         // empty = no subtype dimension
	MaxKeyLen       int              // 0 = no truncation
	MaxLabelLen     int              // 0 = no truncation
	MaxDescLen      int              // 0 = no truncation
	MaxPayloadBytes int              // 0 = no limit check
	DefaultCategory string           // applied on Create when category empty
	Sorter          Sorter           // optional override; nil = default sort
	Seeder          Seeder           // optional; nil = no defaults to import
	PayloadSchema   map[string]any   // optional, for future validation / docs
}

// Sorter reorders List output for a domain. If nil, the Service preserves
// whatever order the Store returned (which is usually repo-defined:
// category ASC, updated_at DESC, name ASC).
type Sorter func(items []Item)

// Seeder returns the default-value inputs to insert on ImportDefaults.
// The same CreateInput shape is reused; UserID is filled in by Service
// from the request.
type Seeder func() []CreateInput

// Registry is the lookup table of registered DomainSpecs. It is immutable
// after construction.
type Registry struct {
	specs   map[Domain]DomainSpec
	ordered []Domain // insertion order, for Domains()
}

// NewRegistry builds a Registry from the given specs. Duplicate domain
// names silently overwrite the earlier entry (last-wins).
func NewRegistry(specs ...DomainSpec) *Registry {
	r := &Registry{
		specs:   make(map[Domain]DomainSpec, len(specs)),
		ordered: make([]Domain, 0, len(specs)),
	}
	for _, s := range specs {
		if _, exists := r.specs[s.Name]; !exists {
			r.ordered = append(r.ordered, s.Name)
		}
		r.specs[s.Name] = s
	}
	return r
}

// Get returns the DomainSpec for d and whether it was registered.
func (r *Registry) Get(d Domain) (DomainSpec, bool) {
	s, ok := r.specs[d]
	return s, ok
}

// Domains returns every registered domain name, in insertion order.
func (r *Registry) Domains() []Domain {
	out := make([]Domain, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// HasSubtype reports whether subtype is a permitted value for this domain.
// A domain with no Subtypes always returns true (any subtype — including
// "" — is accepted; subtype is informational only).
func (s DomainSpec) HasSubtype(subtype string) bool {
	if len(s.Subtypes) == 0 {
		return true
	}
	for _, st := range s.Subtypes {
		if st == subtype {
			return true
		}
	}
	return false
}

// IsReadOnly reports whether this domain rejects Create/Update/Delete
// through the public Catalog API. System-scope domains (e.g.
// tool_definition) are read-only by convention.
func (s DomainSpec) IsReadOnly() bool {
	return s.Scope == ScopeSystem
}
