package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"the-dancing-pony-v2/internal/authz"
	"the-dancing-pony-v2/internal/store/sqlcgen"
)

// Role returns userID's role at a restaurant: authz.Customer when they
// have no membership there.
func (s *Store) Role(ctx context.Context, restaurantID, userID uuid.UUID) (authz.Role, error) {
	var role string
	err := s.inTenant(ctx, restaurantID, pgx.ReadOnly, func(q *sqlcgen.Queries) (err error) {
		role, err = q.GetMembershipRole(ctx, sqlcgen.GetMembershipRoleParams{RestaurantID: restaurantID, UserID: userID})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return authz.Customer, nil
	}
	if err != nil {
		return "", fmt.Errorf("get role: %w", err)
	}
	return authz.Role(role), nil
}

// GrantRole makes the user with email an owner or staff member of the
// restaurant with slug. dsn must connect as the schema owner.
func GrantRole(ctx context.Context, dsn, email, slug string, role authz.Role) error {
	if role != authz.Owner && role != authz.Staff {
		return fmt.Errorf("role must be %s or %s", authz.Owner, authz.Staff)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		userID, err := q.GetUserIDByEmail(ctx, email)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("no user with email %s", email)
		}
		if err != nil {
			return err
		}
		rest, err := q.GetRestaurantBySlug(ctx, slug)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("no restaurant with slug %s", slug)
		}
		if err != nil {
			return err
		}
		// memberships is row-security protected, for the owner too.
		if err := q.SetTenant(ctx, rest.ID); err != nil {
			return err
		}
		return q.UpsertMembership(ctx, sqlcgen.UpsertMembershipParams{RestaurantID: rest.ID, UserID: userID, Role: string(role)})
	})
}
