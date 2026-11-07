package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"the-dancing-pony-v2/internal/auth"
	"the-dancing-pony-v2/internal/authz"
	"the-dancing-pony-v2/internal/store/sqlcgen"
)

// DemoPassword is the password of every demo user. pony seed is for local
// development and demos only; never run it against production.
const DemoPassword = "pony-demo-password" //nolint:gosec // G101: a published demo password, by design

// DemoUser is a demo account. Name is a short handle for tests and Bruno.
// Restaurant and Role, if set, make them owner or staff there.
type DemoUser struct {
	Name, Email string
	Restaurant  string
	Role        authz.Role
}

// DemoUsers are the demo accounts pony seed creates.
var DemoUsers = []DemoUser{
	{Name: "orc-owner", Email: "owner@orc-shack.example", Restaurant: "orc-shack", Role: authz.Owner},
	{Name: "dragon-owner", Email: "owner@green-dragon.example", Restaurant: "green-dragon", Role: authz.Owner},
	{Name: "frodo", Email: "frodo@shire.example"},
	{Name: "sam", Email: "sam@shire.example"},
}

// SeedUsers creates the given users with password pw if they don't exist.
// dsn must connect as the schema owner.
func SeedUsers(ctx context.Context, dsn string, users []DemoUser, pw string) error {
	hash, err := auth.HashPassword(ctx, pw)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	q := sqlcgen.New(conn)
	for _, u := range users {
		if _, err := q.UpsertUser(ctx, sqlcgen.UpsertUserParams{Email: u.Email, PasswordHash: hash}); err != nil {
			return fmt.Errorf("seed user %s: %w", u.Email, err)
		}
	}
	return nil
}

// SeedMemberships gives each demo user with a Restaurant their Role there.
// Run it after the users and restaurants exist.
func SeedMemberships(ctx context.Context, dsn string, users []DemoUser) error {
	for _, u := range users {
		if u.Restaurant == "" {
			continue
		}
		if err := GrantRole(ctx, dsn, u.Email, u.Restaurant, u.Role); err != nil {
			return fmt.Errorf("seed membership %s: %w", u.Email, err)
		}
	}
	return nil
}
