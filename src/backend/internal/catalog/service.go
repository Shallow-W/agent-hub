package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// Service is the application layer above Store. It owns:
//   - domain validation (against Registry)
//   - input normalization (trim, truncate, default category)
//   - error mapping (store-specific → unified catalog sentinels)
//   - default-value seeding via DomainSpec.Seeder
//
// The Service is deliberately domain-agnostic — it never switches on a
// specific Domain value. All variation comes from DomainSpec.
type Service struct {
	store    Store
	registry *Registry
}

// NewService builds a Service bound to the given Store. If the Store also
// exposes its Registry via the Registrar interface (see AdapterStore), it
// will be reused; otherwise pass a Registry explicitly via WithRegistry.
func NewService(store Store, registry *Registry) *Service {
	if registry == nil {
		if r, ok := store.(Registrar); ok {
			registry = r.Registry()
		}
	}
	svc := &Service{store: store, registry: registry}
	// Defensive: if the store didn't carry a registry, synthesize an empty one
	// so callers that don't care about domain validation still work.
	if svc.registry == nil {
		svc.registry = NewRegistry()
	}
	return svc
}

// Registrar is implemented by stores that already hold a Registry (e.g.
// AdapterStore). It lets NewService omit the registry argument.
type Registrar interface {
	Registry() *Registry
}

// Registry exposes the spec registry (handler / external code uses it).
func (s *Service) Registry() *Registry { return s.registry }

// List returns every Item in the given domain matching q.
func (s *Service) List(ctx context.Context, domain Domain, q ListQuery) ([]Item, error) {
	if _, err := s.requireDomain(domain); err != nil {
		return nil, err
	}
	list, err := s.store.List(ctx, domain, q)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	if list == nil {
		return []Item{}, nil
	}
	if spec, ok := s.registry.Get(domain); ok && spec.Sorter != nil {
		spec.Sorter(list)
	}
	return list, nil
}

// Get returns a single Item by id.
func (s *Service) Get(ctx context.Context, id string) (*Item, error) {
	item, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	if item == nil {
		return nil, ErrNotFound
	}
	return item, nil
}

// Create validates + normalizes the input, then delegates to Store. For
// system-scope (read-only) domains it returns ErrReadOnly without touching
// the Store.
func (s *Service) Create(ctx context.Context, input CreateInput) (*Item, error) {
	spec, err := s.requireDomain(input.Domain)
	if err != nil {
		return nil, err
	}
	if spec.IsReadOnly() {
		return nil, fmt.Errorf("%w: %s", ErrReadOnly, input.Domain)
	}
	if err := normalizeCreate(&spec, &input); err != nil {
		return nil, err
	}
	item, err := s.store.Create(ctx, input)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	return item, nil
}

// Update applies a partial update. Pointer-valued fields that are nil are
// left unchanged; non-nil pointers replace the stored value. Domain is read
// from input.Domain (the Handler threads it from the URL path); the Store
// uses it to dispatch to the right repo.
func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (*Item, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrInvalid
	}
	if _, err := s.requireDomain(input.Domain); err != nil {
		return nil, err
	}
	item, err := s.store.Update(ctx, id, input)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	if item == nil {
		return nil, ErrNotFound
	}
	return item, nil
}

// Delete removes an Item by id. domain tells the Store which repo to use;
// userID is required for user-scope domains (the underlying repo enforces it
// to prevent cross-user deletion); system-scope domains pass empty string.
func (s *Service) Delete(ctx context.Context, domain Domain, userID, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrInvalid
	}
	if _, err := s.requireDomain(domain); err != nil {
		return err
	}
	if err := s.store.Delete(ctx, domain, userID, id); err != nil {
		return mapStoreErr(err)
	}
	return nil
}

// ImportDefaults invokes the domain's Seeder (if registered) and creates
// each returned input via Create. Items whose key already exists return
// ErrDuplicate from the Store; ImportDefaults treats that as a soft skip
// (idempotent re-import) rather than failing.
func (s *Service) ImportDefaults(ctx context.Context, domain Domain, userID string) ([]Item, error) {
	spec, err := s.requireDomain(domain)
	if err != nil {
		return nil, err
	}
	if spec.IsReadOnly() {
		return nil, fmt.Errorf("%w: %s", ErrReadOnly, domain)
	}
	if spec.Seeder == nil {
		return []Item{}, nil
	}
	if spec.Scope == ScopeUser && strings.TrimSpace(userID) == "" {
		return nil, ErrInvalid
	}
	imported := make([]Item, 0, 8)
	for _, in := range spec.Seeder() {
		in.Domain = domain
		if spec.Scope == ScopeUser {
			in.UserID = userID
		}
		item, createErr := s.Create(ctx, in)
		if createErr != nil {
			if errors.Is(createErr, ErrDuplicate) {
				continue
			}
			return imported, createErr
		}
		imported = append(imported, *item)
	}
	return imported, nil
}

// requireDomain returns the spec for d, or ErrUnknownDomain if it isn't
// registered.
func (s *Service) requireDomain(d Domain) (DomainSpec, error) {
	if s.registry == nil {
		return DomainSpec{}, ErrUnknownDomain
	}
	spec, ok := s.registry.Get(d)
	if !ok {
		return DomainSpec{}, fmt.Errorf("%w: %s", ErrUnknownDomain, d)
	}
	return spec, nil
}

// normalizeCreate applies DomainSpec limits: trim whitespace, truncate over
// long values, fill DefaultCategory when missing, enforce subtype allowlist.
func normalizeCreate(spec *DomainSpec, in *CreateInput) error {
	in.Key = strings.TrimSpace(in.Key)
	in.Label = strings.TrimSpace(in.Label)
	in.Description = strings.TrimSpace(in.Description)
	in.Category = strings.TrimSpace(in.Category)
	in.Subtype = strings.TrimSpace(in.Subtype)

	if in.Key == "" {
		return fmt.Errorf("%w: key is empty", ErrInvalid)
	}
	if in.Label == "" {
		in.Label = in.Key // fall back to Key for Label
	}
	if spec.MaxKeyLen > 0 {
		in.Key = truncateRunes(in.Key, spec.MaxKeyLen)
	}
	if spec.MaxLabelLen > 0 {
		in.Label = truncateRunes(in.Label, spec.MaxLabelLen)
	}
	if spec.MaxDescLen > 0 {
		in.Description = truncateRunes(in.Description, spec.MaxDescLen)
	}
	if in.Category == "" {
		in.Category = spec.DefaultCategory
	}
	if !spec.HasSubtype(in.Subtype) {
		return fmt.Errorf("%w: subtype %q not allowed for %s", ErrInvalid, in.Subtype, spec.Name)
	}
	if spec.MaxPayloadBytes > 0 && len(in.PayloadJSON) > spec.MaxPayloadBytes {
		return fmt.Errorf("%w: payload exceeds %d bytes", ErrInvalid, spec.MaxPayloadBytes)
	}
	return nil
}

// truncateRunes cuts s to maxRunes runes (UTF-8 safe). Mirrors
// service.truncateString behavior but local to the catalog package so the
// catalog has no inbound dependency on internal/service.
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// mapStoreErr collapses a Store-layer error onto the unified catalog
// sentinels. Unknown errors pass through unchanged.
func mapStoreErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrDuplicate) ||
		errors.Is(err, ErrUnknownDomain) ||
		errors.Is(err, ErrReadOnly) {
		return err
	}
	// Heuristic: sql.ErrNoRows → ErrNotFound (AdapterStore may also do this).
	if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no rows") {
		return ErrNotFound
	}
	// PostgreSQL unique-violation (SQLSTATE 23505) → ErrDuplicate. Serves
	// every domain since the underlying repos all surface pgconn.PgError
	// when the (domain, user_id, key) uniqueness constraint fires.
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

// isUniqueViolation mirrors service.isUniqueViolation without creating a
// catalog → service dependency (which would close the import cycle
// catalog → middleware → service → catalog).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
