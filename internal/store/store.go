package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"the-dancing-pony-v2/internal/store/sqlcgen"
	"the-dancing-pony-v2/internal/tenant"
)

// ErrNotFound is returned when a requested row does not exist (or, under
// Row-Level Security, is not visible to the current tenant).
var ErrNotFound = errors.New("not found")

// ErrPrivilegedRole is returned by Open when the database role could see
// past Row-Level Security.
var ErrPrivilegedRole = errors.New("database role can bypass row-level security")

// Store is the API's access to Postgres. It connects as pony_app.
type Store struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// Open connects to Postgres, checks the connection works, and refuses to
// continue as a role that Row-Level Security would not restrict.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := checkUnprivileged(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool, q: sqlcgen.New(pool)}, nil
}

// checkUnprivileged fails if the connected role is a superuser, has
// BYPASSRLS, or owns (or is a member of the owner of) a table with RLS
// enabled; an owner could also drop the policies. Plain pgx rather than
// sqlc, which doesn't model pg_roles or pg_class.
func checkUnprivileged(ctx context.Context, pool *pgxpool.Pool) error {
	var (
		role                      string
		super, bypass, ownsRLSTab bool
	)
	err := pool.QueryRow(ctx, `
		SELECT current_user::text, r.rolsuper, r.rolbypassrls,
		       EXISTS (SELECT 1 FROM pg_class c
		               WHERE c.relrowsecurity
		                 AND pg_has_role(current_user, c.relowner, 'USAGE'))
		FROM pg_roles r
		WHERE r.rolname = current_user`).Scan(&role, &super, &bypass, &ownsRLSTab)
	if err != nil {
		return fmt.Errorf("check database role: %w", err)
	}
	switch {
	case super:
		return fmt.Errorf("%w: %s is a superuser", ErrPrivilegedRole, role)
	case bypass:
		return fmt.Errorf("%w: %s has BYPASSRLS", ErrPrivilegedRole, role)
	case ownsRLSTab:
		return fmt.Errorf("%w: %s owns tables; connect as pony_app", ErrPrivilegedRole, role)
	}
	return nil
}

// inTenant runs fn in a transaction that Row-Level Security scopes to one
// restaurant. The setting ends with the transaction, so a pooled
// connection never carries one request's tenant into another.
func (s *Store) inTenant(ctx context.Context, restaurantID uuid.UUID, access pgx.TxAccessMode,
	fn func(*sqlcgen.Queries) error,
) error {
	return pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: access}, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.SetTenant(ctx, restaurantID); err != nil {
			return fmt.Errorf("set tenant: %w", err)
		}
		return fn(q)
	})
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
