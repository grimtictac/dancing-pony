package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"the-dancing-pony-v2/internal/store/sqlcgen"
	"the-dancing-pony-v2/internal/tenant"
)

// ErrNotFound is returned when a requested row does not exist (or, under
// Row-Level Security, is not visible to the current tenant).
var ErrNotFound = errors.New("not found")

// Store is the API's access to Postgres. It connects as pony_app.
type Store struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// Open connects to Postgres and checks the connection works.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return &Store{pool: pool, q: sqlcgen.New(pool)}, nil
}

// Close closes every connection in the pool.
func (s *Store) Close() {
	s.pool.Close()
}

// RestaurantBySlug returns the restaurant with the given slug.
func (s *Store) RestaurantBySlug(ctx context.Context, slug string) (tenant.Restaurant, error) {
	row, err := s.q.GetRestaurantBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenant.Restaurant{}, ErrNotFound
	}
	if err != nil {
		return tenant.Restaurant{}, fmt.Errorf("get restaurant %q: %w", slug, err)
	}
	return tenant.Restaurant{ID: row.ID, Slug: row.Slug, Name: row.Name, Currency: row.Currency}, nil
}
