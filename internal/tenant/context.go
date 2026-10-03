package tenant

import "context"

// DefaultID is the tenant that owns every row in single-tenant mode, and the
// tenant pre-existing data was assigned to when tenants were introduced.
const DefaultID = 1

type ctxKey struct{}

// WithID returns a copy of ctx carrying the tenant ID.
func WithID(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// IDFromContext returns the tenant ID carried by ctx, or 0 if there is none.
func IDFromContext(ctx context.Context) int {
	id, _ := ctx.Value(ctxKey{}).(int)
	return id
}
