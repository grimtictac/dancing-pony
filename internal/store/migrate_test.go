package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"

	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/store/storetest"
)

// Every migration must apply, roll back and apply again cleanly, so a
// broken Down section is caught before it is needed in an emergency.
func TestMigrationsUpDownUp(t *testing.T) {
	ctx := context.Background()
	db := openDB(t, storetest.Empty(t).OwnerURL)
	p, err := store.NewMigrator(db)
	if err != nil {
		t.Fatal(err)
	}
	total := len(p.ListSources())
	if total == 0 {
		t.Fatal("no migrations embedded")
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}
	assertAllInState(t, p, goose.StateApplied)

	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("down to 0: %v", err)
	}
	assertAllInState(t, p, goose.StatePending)

	results, err := p.Up(ctx)
	if err != nil {
		t.Fatalf("up again: %v", err)
	}
	if len(results) != total {
		t.Errorf("second up applied %d migrations, want %d", len(results), total)
	}
}

func TestAppRoleCannotBypassRowLevelSecurity(t *testing.T) {
	db := openDB(t, storetest.Migrated(t).OwnerURL)

	var super, bypass, createRole, createDB bool
	err := db.QueryRowContext(context.Background(),
		`SELECT rolsuper, rolbypassrls, rolcreaterole, rolcreatedb
		   FROM pg_roles WHERE rolname = 'pony_app'`).
		Scan(&super, &bypass, &createRole, &createDB)
	if err != nil {
		t.Fatal(err)
	}
	if super || bypass || createRole || createDB {
		t.Errorf("pony_app: superuser=%v bypassrls=%v createrole=%v createdb=%v, want all false",
			super, bypass, createRole, createDB)
	}
}

func assertAllInState(t *testing.T, p *goose.Provider, want goose.State) {
	t.Helper()
	statuses, err := p.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if s.State != want {
			t.Errorf("migration %d is %s, want %s", s.Source.Version, s.State, want)
		}
	}
}

func openDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
