// Package store is the Postgres persistence layer: embedded migrations,
// the connection pool and tenant-scoped queries.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedded embed.FS

// Migrations returns the SQL migrations built into the binary.
func Migrations() fs.FS {
	sub, err := fs.Sub(embedded, "migrations")
	if err != nil {
		panic(err) // the path is a compile-time constant
	}
	return sub
}

// NewMigrator returns a goose provider for the built-in migrations. db must
// connect as the role that owns the schema, not as pony_app.
func NewMigrator(db *sql.DB) (*goose.Provider, error) {
	p, err := goose.NewProvider(goose.DialectPostgres, db, Migrations())
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	return p, nil
}
