// Package storetest provides real Postgres databases for tests.
//
// One container is started per test binary, on first use, and removed by
// testcontainers when the binary exits. Each test gets its own database.
// Migrations run as pony_owner, a non-superuser role with CREATEROLE, like
// the admin user of a managed database, so tests exercise the same
// permissions as production.
package storetest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	tclog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"the-dancing-pony-v2/internal/store"
)

// Image is the Postgres image used in tests. Keep it in step with
// docker-compose.yml.
const Image = "postgres:18.6-alpine@sha256:123b17998ccaf47e53b502e8c6a1e79afd8f579c6f3eec83c563da026f957395"

const (
	ownerRole     = "pony_owner"
	ownerPassword = "owner"
	appRole       = "pony_app"
	appPassword   = "app"
	templateDB    = "pony_template"
)

// DB is a test database.
type DB struct {
	Name string
	// OwnerURL connects as pony_owner, which owns the schema and runs
	// migrations.
	OwnerURL string
	// AppURL connects as pony_app, the API's role. It is only usable after
	// migrations have created the role.
	AppURL string
	// SuperURL connects as a superuser, which bypasses Row-Level Security.
	SuperURL string
}

var (
	startOnce sync.Once
	superURL  *url.URL
	startErr  error

	templateOnce sync.Once
	templateErr  error

	cloneMu sync.Mutex // CREATE DATABASE ... TEMPLATE needs the template idle
	counter atomic.Int64
)

// Empty returns a new database with no migrations applied.
func Empty(t testing.TB) DB {
	t.Helper()
	ensureStarted(t)
	return create(t, "")
}

// Migrated returns a new database with every migration applied. It is cloned
// from a template, so it is cheap to call from every test.
func Migrated(t testing.TB) DB {
	t.Helper()
	ensureStarted(t)
	templateOnce.Do(func() { templateErr = buildTemplate() })
	if templateErr != nil {
		t.Fatalf("storetest: migrate template database: %v", templateErr)
	}
	return create(t, templateDB)
}

func ensureStarted(t testing.TB) {
	t.Helper()
	if testing.Short() {
		t.Skip("storetest: needs Docker; skipped with -short")
	}
	startOnce.Do(func() { startErr = start() })
	if startErr != nil {
		t.Fatalf("storetest: start Postgres: %v", startErr)
	}
}

func start() error {
	// Container progress logs would drown out test output; errors are
	// still returned.
	tclog.SetDefault(quietLogger{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c, err := postgres.Run(ctx, Image,
		postgres.WithDatabase("postgres"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return err
	}
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return err
	}
	if superURL, err = url.Parse(dsn); err != nil {
		return err
	}
	return execAsSuperuser(ctx,
		fmt.Sprintf("CREATE ROLE %s LOGIN CREATEROLE PASSWORD '%s'", ownerRole, ownerPassword))
}

func buildTemplate() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := execAsSuperuser(ctx, fmt.Sprintf("CREATE DATABASE %s OWNER %s", templateDB, ownerRole)); err != nil {
		return err
	}
	db, err := sql.Open("pgx", urlFor(templateDB, ownerRole, ownerPassword))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	p, err := store.NewMigrator(db)
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return err
	}
	// Production sets a real password for pony_app out of band.
	return execAsSuperuser(ctx, fmt.Sprintf("ALTER ROLE %s LOGIN PASSWORD '%s'", appRole, appPassword))
}

func create(t testing.TB, template string) DB {
	t.Helper()
	ctx := context.Background()
	name := fmt.Sprintf("test_%d", counter.Add(1))

	stmt := fmt.Sprintf("CREATE DATABASE %s OWNER %s", name, ownerRole)
	if template != "" {
		stmt += " TEMPLATE " + template
	}
	cloneMu.Lock()
	err := execAsSuperuser(ctx, stmt)
	cloneMu.Unlock()
	if err != nil {
		t.Fatalf("storetest: %v", err)
	}
	t.Cleanup(func() {
		if err := execAsSuperuser(context.Background(),
			fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", name)); err != nil {
			t.Errorf("storetest: drop %s: %v", name, err)
		}
	})

	return DB{
		Name:     name,
		OwnerURL: urlFor(name, ownerRole, ownerPassword),
		AppURL:   urlFor(name, appRole, appPassword),
		SuperURL: urlFor(name, superURL.User.Username(), superPassword()),
	}
}

func urlFor(dbName, user, password string) string {
	u := *superURL
	u.User = url.UserPassword(user, password)
	u.Path = "/" + dbName
	return u.String()
}

func execAsSuperuser(ctx context.Context, stmt string) error {
	db, err := sql.Open("pgx", superURL.String())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("%s: %w", stmt, err)
	}
	return nil
}

type quietLogger struct{}

func (quietLogger) Printf(string, ...any) {}

func superPassword() string {
	p, _ := superURL.User.Password()
	return p
}
