package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"the-dancing-pony-v2/internal/store"
)

const seedUsage = `Usage: pony seed

Adds the demo users, the demo restaurants (orc-shack and green-dragon) and
their starting dishes. A restaurant that already has dishes is left alone, so it is safe
to run repeatedly. For development and demos, not production.

Connects with PONY_MIGRATE_DATABASE_URL: the role that owns the schema.
`

func seed(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
			_, _ = fmt.Fprint(stdout, seedUsage)
			return 0
		}
		_, _ = fmt.Fprint(stderr, seedUsage)
		return 2
	}
	dsn := getenv("PONY_MIGRATE_DATABASE_URL")
	if dsn == "" {
		_, _ = fmt.Fprintln(stderr, "pony seed: PONY_MIGRATE_DATABASE_URL is not set")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := store.SeedUsers(ctx, dsn, store.DemoUsers, store.DemoPassword); err != nil {
		_, _ = fmt.Fprintf(stderr, "pony seed: %v\n", err)
		return 1
	}
	for _, u := range store.DemoUsers {
		_, _ = fmt.Fprintf(stdout, "user %s (password %q)\n", u.Email, store.DemoPassword)
	}
	results, err := store.Seed(ctx, dsn, store.DemoRestaurants)
	for _, r := range results {
		_, _ = fmt.Fprintf(stdout, "%s: dishes added: %d\n", r.Slug, r.DishesAdded)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "pony seed: %v\n", err)
		return 1
	}
	if err := store.SeedMemberships(ctx, dsn, store.DemoUsers); err != nil {
		_, _ = fmt.Fprintf(stderr, "pony seed: %v\n", err)
		return 1
	}
	return 0
}
