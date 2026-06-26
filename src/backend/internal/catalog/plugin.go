package catalog

import "context"

// DomainPlugin encapsulates all domain-specific behavior that AdapterStore
// previously handled via switch statements. Each domain registers one plugin;
// AdapterStore dispatches by looking up the plugin in its registry.
//
// To add a new catalog domain: implement this interface, register it via
// AdapterDeps.Plugins[DomainX], and add a DomainSpec to DefaultRegistry.
// You do NOT need to modify adapter.go, service.go, or handler.go.
type DomainPlugin interface {
	// List returns all items for the given user. For system-scope plugins,
	// userID is empty and should be ignored.
	List(ctx context.Context, userID string) ([]Item, error)

	// Create persists a new item and returns the saved form.
	Create(ctx context.Context, input CreateInput) (*Item, error)

	// Update applies a partial update. The plugin should list-then-match
	// to resolve current values for nil pointers in the input, then
	// forward the fully-populated update to the underlying repo.
	Update(ctx context.Context, id, userID string, input UpdateInput) (*Item, error)

	// Delete removes the item. Returns ErrNotFound if the item didn't exist.
	Delete(ctx context.Context, id, userID string) error
}
