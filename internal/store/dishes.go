package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/store/sqlcgen"
)

// GetDish returns one of a restaurant's dishes, or ErrNotFound.
func (s *Store) GetDish(ctx context.Context, restaurantID, dishID uuid.UUID) (dish.Dish, error) {
	var r dishRow
	err := s.inTenantTx(ctx, restaurantID, pgx.ReadOnly, func(tx pgx.Tx) (err error) {
		r, err = getDishRow(ctx, tx, restaurantID, dishID)
		return err
	})
	return rowResult(r, err, "get dish")
}

// CreateDish adds a dish to a restaurant. Any ID in d is ignored.
func (s *Store) CreateDish(ctx context.Context, restaurantID uuid.UUID, d dish.Dish) (dish.Dish, error) {
	var r dishRow
	err := s.inTenantTx(ctx, restaurantID, pgx.ReadWrite, func(tx pgx.Tx) error {
		dishID, err := s.q.WithTx(tx).CreateDish(ctx, sqlcgen.CreateDishParams{
			RestaurantID:  restaurantID,
			Name:          d.Name,
			Description:   d.Description,
			PriceAmount:   d.Price.Amount,
			PriceCurrency: d.Price.Currency,
			ImageUrl:      d.Image,
		})
		if err != nil {
			return err
		}
		// Read it back like every other dish, in the same transaction.
		r, err = getDishRow(ctx, tx, restaurantID, dishID)
		return err
	})
	return rowResult(r, err, "create dish")
}

// UpdateDish replaces one of a restaurant's dishes, or returns ErrNotFound.
// Any ID in d is ignored.
func (s *Store) UpdateDish(ctx context.Context, restaurantID, dishID uuid.UUID, d dish.Dish) (dish.Dish, error) {
	var r dishRow
	err := s.inTenantTx(ctx, restaurantID, pgx.ReadWrite, func(tx pgx.Tx) error {
		_, err := s.q.WithTx(tx).UpdateDish(ctx, sqlcgen.UpdateDishParams{
			RestaurantID:  restaurantID,
			ID:            dishID,
			Name:          d.Name,
			Description:   d.Description,
			PriceAmount:   d.Price.Amount,
			PriceCurrency: d.Price.Currency,
			ImageUrl:      d.Image,
		})
		if err != nil {
			return err
		}
		// Read it back with its rating summary, in the same transaction.
		r, err = getDishRow(ctx, tx, restaurantID, dishID)
		return err
	})
	return rowResult(r, err, "update dish")
}

// DeleteDish removes one of a restaurant's dishes, or returns ErrNotFound.
//
// It returns the key of the dish's photo ("" if none), for the caller to
// delete from photo storage once this has committed.
func (s *Store) DeleteDish(ctx context.Context, restaurantID, dishID uuid.UUID) (photoKey string, err error) {
	var n int64
	err = s.inTenant(ctx, restaurantID, pgx.ReadWrite, func(q *sqlcgen.Queries) (err error) {
		photoKey, err = q.DeleteDishPhoto(ctx, sqlcgen.DeleteDishPhotoParams{RestaurantID: restaurantID, DishID: dishID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		n, err = q.DeleteDish(ctx, sqlcgen.DeleteDishParams{RestaurantID: restaurantID, ID: dishID})
		return err
	})
	if err != nil {
		return "", fmt.Errorf("delete dish: %w", err)
	}
	if n == 0 {
		return "", ErrNotFound
	}
	return photoKey, nil
}

func rowResult(r dishRow, err error, op string) (dish.Dish, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return dish.Dish{}, ErrNotFound
	}
	if err != nil {
		return dish.Dish{}, fmt.Errorf("%s: %w", op, err)
	}
	return r.toDish(), nil
}
