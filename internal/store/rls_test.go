package store_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/store/storetest"
)

// These tests talk to the database directly, with queries that leave out
// "WHERE restaurant_id = ...", to show that Row-Level Security alone keeps
// restaurants apart.

func TestRowLevelSecurity(t *testing.T) {
	ctx := context.Background()
	db := storetest.Migrated(t)
	owner := openDB(t, db.OwnerURL)
	shack := addRestaurant(t, owner, "orc-shack", "ZAR")
	dragon := addRestaurant(t, owner, "green-dragon", "ZAR")
	s := openStore(t, db.AppURL)
	for _, r := range []uuid.UUID{shack, dragon} {
		for range 2 {
			if _, err := s.CreateDish(ctx, r, lembas(4550)); err != nil {
				t.Fatal(err)
			}
		}
	}
	app := openDB(t, db.AppURL)

	t.Run("no tenant set sees nothing", func(t *testing.T) {
		if n := count(t, app, "SELECT count(*) FROM dishes"); n != 0 {
			t.Errorf("saw %d dishes without a tenant, want 0", n)
		}
	})

	t.Run("a tenant sees only its own rows", func(t *testing.T) {
		inTenant(ctx, t, app, shack, func(tx *sql.Tx) {
			if n := count(t, tx, "SELECT count(*) FROM dishes"); n != 2 {
				t.Errorf("orc-shack saw %d dishes, want its own 2", n)
			}
			if n := count(t, tx, "SELECT count(*) FROM dishes WHERE restaurant_id = $1", dragon); n != 0 {
				t.Errorf("orc-shack saw %d of green-dragon's dishes, want 0", n)
			}
		})
	})

	t.Run("cannot insert into another tenant", func(t *testing.T) {
		inTenant(ctx, t, app, shack, func(tx *sql.Tx) {
			_, err := tx.ExecContext(ctx, `INSERT INTO dishes
				(restaurant_id, name, description, price_amount, price_currency, image_url)
				VALUES ($1, 'Stolen', 'x', 1, 'ZAR', 'x')`, dragon)
			wantPolicyViolation(t, err)
		})
	})

	t.Run("cannot move a row to another tenant", func(t *testing.T) {
		inTenant(ctx, t, app, shack, func(tx *sql.Tx) {
			_, err := tx.ExecContext(ctx, `UPDATE dishes SET restaurant_id = $1`, dragon)
			wantPolicyViolation(t, err)
		})
	})

	t.Run("unscoped update and delete only touch the tenant's rows", func(t *testing.T) {
		inTenant(ctx, t, app, shack, func(tx *sql.Tx) {
			if n := exec(t, tx, "UPDATE dishes SET name = 'Renamed'"); n != 2 {
				t.Errorf("updated %d rows, want orc-shack's 2", n)
			}
			if n := exec(t, tx, "DELETE FROM dishes"); n != 2 {
				t.Errorf("deleted %d rows, want orc-shack's 2", n)
			}
		})
		inTenant(ctx, t, app, dragon, func(tx *sql.Tx) {
			if n := count(t, tx, "SELECT count(*) FROM dishes WHERE name = 'Lembas'"); n != 2 {
				t.Errorf("green-dragon has %d untouched dishes, want 2", n)
			}
		})
	})

	t.Run("the setting ends with the transaction", func(t *testing.T) {
		conn, err := app.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		setTenant(ctx, t, tx, dragon)
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		// Same connection, as a pool would reuse it for the next request.
		var n int
		if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM dishes").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("saw %d dishes after the tenant's transaction ended, want 0", n)
		}
	})

	t.Run("FORCE applies the policy to the table owner too", func(t *testing.T) {
		if n := count(t, owner, "SELECT count(*) FROM dishes"); n != 0 {
			t.Errorf("owner saw %d dishes without a tenant, want 0", n)
		}
	})
}

func TestOpenRefusesRolesThatBypassRowLevelSecurity(t *testing.T) {
	db := storetest.Migrated(t)
	for name, dsn := range map[string]string{
		"table owner": db.OwnerURL,
		"superuser":   db.SuperURL,
	} {
		t.Run(name, func(t *testing.T) {
			s, err := store.Open(context.Background(), dsn)
			if err == nil {
				s.Close()
			}
			if !errors.Is(err, store.ErrPrivilegedRole) {
				t.Errorf("Open as %s: err = %v, want ErrPrivilegedRole", name, err)
			}
		})
	}
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func count(t *testing.T, q queryer, query string, args ...any) int {
	t.Helper()
	var n int
	if err := q.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func exec(t *testing.T, tx *sql.Tx, query string) int64 {
	t.Helper()
	res, err := tx.ExecContext(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// inTenant runs fn in a transaction scoped to restaurantID, then rolls it
// back so subtests don't affect each other.
func inTenant(ctx context.Context, t *testing.T, db *sql.DB, restaurantID uuid.UUID, fn func(*sql.Tx)) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	setTenant(ctx, t, tx, restaurantID)
	fn(tx)
}

func setTenant(ctx context.Context, t *testing.T, tx *sql.Tx, restaurantID uuid.UUID) {
	t.Helper()
	if _, err := tx.ExecContext(ctx,
		"SELECT set_config('app.restaurant_id', $1, true)", restaurantID.String()); err != nil {
		t.Fatal(err)
	}
}

func wantPolicyViolation(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("write succeeded, want a row-level security violation")
	}
	// 42501 insufficient_privilege: "new row violates row-level security policy"
	const want = "row-level security policy"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to mention %q", err, want)
	}
}
