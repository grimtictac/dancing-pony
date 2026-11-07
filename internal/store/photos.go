package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"the-dancing-pony-v2/internal/photo"
	"the-dancing-pony-v2/internal/store/sqlcgen"
)

// SetDishPhoto records p, stored under key, as the dish's photo. It returns
// the key of the photo it replaced ("" if none), for the caller to delete
// once this has committed; or ErrNotFound if there is no such dish.
func (s *Store) SetDishPhoto(ctx context.Context, restaurantID, dishID uuid.UUID, key string, p photo.Processed) (string, error) {
	var old string
	err := s.inTenant(ctx, restaurantID, pgx.ReadWrite, func(q *sqlcgen.Queries) error {
		var err error
		old, err = q.GetDishPhotoKey(ctx, sqlcgen.GetDishPhotoKeyParams{RestaurantID: restaurantID, DishID: dishID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return q.UpsertDishPhoto(ctx, sqlcgen.UpsertDishPhotoParams{
			RestaurantID: restaurantID,
			DishID:       dishID,
			Key:          key,
			Width:        int32(p.Width),  //nolint:gosec // G115: at most photo.MaxSide
			Height:       int32(p.Height), //nolint:gosec // G115: at most photo.MaxSide
			Thumbhash:    p.ThumbHash,
			DataUrl:      p.DataURL,
		})
	})
	// The composite foreign key: no such dish in this restaurant.
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgForeignKeyViolation {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("set dish photo: %w", err)
	}
	return old, nil
}

// DeleteDishPhoto removes the dish's photo and returns its key, for the
// caller to delete from photo storage; or ErrNotFound if it has none.
func (s *Store) DeleteDishPhoto(ctx context.Context, restaurantID, dishID uuid.UUID) (string, error) {
	var key string
	err := s.inTenant(ctx, restaurantID, pgx.ReadWrite, func(q *sqlcgen.Queries) (err error) {
		key, err = q.DeleteDishPhoto(ctx, sqlcgen.DeleteDishPhotoParams{RestaurantID: restaurantID, DishID: dishID})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("delete dish photo: %w", err)
	}
	return key, nil
}
