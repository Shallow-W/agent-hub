package catalog

import "context"

// Store is the storage abstraction used by Service. The concrete
// implementation in adapter.go proxies to the four existing repositories
// without touching their databases; a future SQL-backed single-table
// implementation can replace it transparently.
//
// Methods are domain-aware: List takes a domain, Create/Update/Delete carry
// the domain via their inputs/IDs so a single Store can serve every
// registered domain.
type Store interface {
	List(ctx context.Context, domain Domain, q ListQuery) ([]Item, error)
	GetByID(ctx context.Context, id string) (*Item, error)
	Create(ctx context.Context, input CreateInput) (*Item, error)
	Update(ctx context.Context, id string, input UpdateInput) (*Item, error)
	Delete(ctx context.Context, domain Domain, userID, id string) error
}
