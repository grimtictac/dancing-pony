// Package tenant holds the current restaurant (tenant) of a request.
package tenant

import (
	"context"

	"github.com/google/uuid"
)

// Restaurant is a tenant of the platform.
type Restaurant struct {
	ID   uuid.UUID
	Slug string
	Name string
	// Currency is the ISO 4217 code all of this restaurant's prices use.
	Currency string
}

type ctxKey struct{}

// WithRestaurant returns a copy of ctx carrying r.
func WithRestaurant(ctx context.Context, r Restaurant) context.Context {
	return context.WithValue(ctx, ctxKey{}, r)
}

// FromContext returns the restaurant stored in ctx, if any.
func FromContext(ctx context.Context) (Restaurant, bool) {
	r, ok := ctx.Value(ctxKey{}).(Restaurant)
	return r, ok
}
