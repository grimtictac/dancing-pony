package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/google/uuid"

	"the-dancing-pony-v2/internal/auth"
	"the-dancing-pony-v2/internal/store/sqlcgen"
)

// ErrConflict is returned when a row would duplicate a unique value, such
// as registering an email that is already taken.
var ErrConflict = errors.New("already exists")

const pgUniqueViolation = "23505"

// CreateUser registers a user. email must already be normalised.
func (s *Store) CreateUser(ctx context.Context, name, email, passwordHash string) (auth.User, error) {
	row, err := s.q.CreateUser(ctx, sqlcgen.CreateUserParams{Name: name, Email: email, PasswordHash: passwordHash})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return auth.User{}, ErrConflict
	}
	if err != nil {
		return auth.User{}, fmt.Errorf("create user: %w", err)
	}
	return auth.User{ID: row.ID, Name: row.Name, Email: row.Email, PlatformAdmin: row.IsPlatformAdmin}, nil
}

// UserByEmail returns the user with this (normalised) email and their
// password hash, or ErrNotFound.
func (s *Store) UserByEmail(ctx context.Context, email string) (auth.User, string, error) {
	row, err := s.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, "", ErrNotFound
	}
	if err != nil {
		return auth.User{}, "", fmt.Errorf("get user: %w", err)
	}
	return auth.User{ID: row.ID, Name: row.Name, Email: row.Email, PlatformAdmin: row.IsPlatformAdmin}, row.PasswordHash, nil
}

// CreateSession stores the hash of a new session token for a user.
func (s *Store) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expires time.Time) error {
	if err := s.q.CreateSession(ctx, sqlcgen.CreateSessionParams{TokenHash: tokenHash, UserID: userID, ExpiresAt: expires}); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// UserBySession returns the user of an unexpired session, or ErrNotFound.
func (s *Store) UserBySession(ctx context.Context, tokenHash []byte) (auth.User, error) {
	row, err := s.q.GetSessionUser(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, ErrNotFound
	}
	if err != nil {
		return auth.User{}, fmt.Errorf("get session: %w", err)
	}
	return auth.User{ID: row.ID, Name: row.Name, Email: row.Email, PlatformAdmin: row.IsPlatformAdmin}, nil
}

// DeleteSession ends a session (logout). Ending one that doesn't exist is
// not an error.
func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if err := s.q.DeleteSession(ctx, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
