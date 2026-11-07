package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"the-dancing-pony-v2/internal/store"
)

const migrateUsage = `Usage: pony migrate [up|down|status]

  up      apply all pending migrations (default)
  down    roll back the most recent migration
  status  list migrations and whether each is applied

Connects with PONY_MIGRATE_DATABASE_URL: the role that owns the schema,
not the API's pony_app role.
`

func migrate(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	action := "up"
	switch len(args) {
	case 0:
	case 1:
		action = args[0]
	default:
		_, _ = fmt.Fprint(stderr, migrateUsage)
		return 2
	}
	switch action {
	case "up", "down", "status":
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(stdout, migrateUsage)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "pony migrate: unknown action %q\n\n%s", action, migrateUsage)
		return 2
	}

	dsn := getenv("PONY_MIGRATE_DATABASE_URL")
	if dsn == "" {
		_, _ = fmt.Fprintln(stderr, "pony migrate: PONY_MIGRATE_DATABASE_URL is not set")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runMigrate(ctx, action, dsn, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "pony migrate %s: %v\n", action, err)
		return 1
	}
	return 0
}

func runMigrate(ctx context.Context, action, dsn string, stdout io.Writer) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	p, err := store.NewMigrator(db)
	if err != nil {
		return err
	}

	switch action {
	case "up":
		results, err := p.Up(ctx)
		for _, r := range results {
			_, _ = fmt.Fprintln(stdout, r)
		}
		if err == nil && len(results) == 0 {
			_, _ = fmt.Fprintln(stdout, "no pending migrations")
		}
		return err
	case "down":
		r, err := p.Down(ctx)
		if r != nil {
			_, _ = fmt.Fprintln(stdout, r)
		}
		return err
	default: // status
		statuses, err := p.Status(ctx)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "VERSION\tSTATE\tAPPLIED AT\tFILE")
		for _, s := range statuses {
			applied := "-"
			if !s.AppliedAt.IsZero() {
				applied = s.AppliedAt.UTC().Format(time.RFC3339)
			}
			_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", s.Source.Version, s.State, applied, s.Source.Path)
		}
		return tw.Flush()
	}
}
